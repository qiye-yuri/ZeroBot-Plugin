package updatewatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Fetcher struct {
	Client   *http.Client
	CacheDir string
}

func NewFetcher() *Fetcher {
	return &Fetcher{CacheDir: filepath.Join(".local", "updatewatch", "source-cache"), Client: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("重定向过多")
		}
		return allowedURL(r.URL.String())
	}}}
}
func (f *Fetcher) get(ctx context.Context, u string) ([]byte, error) {
	if err := allowedURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZeroBot-UpdateWatch/1.0 (public release monitor)")
	r, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("公开网页请求失败")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", r.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20+1))
	if len(b) > 4<<20 {
		return nil, fmt.Errorf("来源响应过大")
	}
	return b, err
}
func (f *Fetcher) json(ctx context.Context, u string, v any) error {
	b, e := f.get(ctx, u)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func plain(s string) string {
	d, e := goquery.NewDocumentFromReader(strings.NewReader(s))
	if e != nil {
		return ""
	}
	d.Find("script,style,nav,header,footer").Remove()
	return strings.Join(strings.Fields(d.Text()), " ")
}
func date(t int64) string { return time.Unix(t, 0).UTC().Format("2006-01-02") }

const ftbPage = "https://www.feed-the-beast.com/modpacks/135-ftb-evolution-2/versions"
const cfBase = "https://www.curseforge.com/minecraft/modpacks/ftb-evolution-2/files"

func (f *Fetcher) ftb(ctx context.Context) ([]Event, error) {
	var pack struct {
		Versions []struct {
			ID         int
			Name, Type string
			Released   int64
			Private    bool
		}
	}
	if e := f.json(ctx, "https://api.feed-the-beast.com/v1/modpacks/public/modpack/135", &pack); e != nil {
		return nil, e
	}
	sort.Slice(pack.Versions, func(i, j int) bool { return pack.Versions[i].Released > pack.Versions[j].Released })
	if len(pack.Versions) == 0 {
		return nil, fmt.Errorf("官网版本列表为空")
	}
	out := []Event{}
	for i, v := range pack.Versions {
		if i >= 20 {
			break
		}
		if v.Private {
			continue
		}
		u := fmt.Sprintf("https://api.feed-the-beast.com/v1/modpacks/public/modpack/135/%d/changelog", v.ID)
		var log struct {
			Content string
			Status  string
		}
		if e := f.json(ctx, u, &log); e != nil {
			return nil, e
		}
		if strings.TrimSpace(log.Content) == "" {
			return nil, fmt.Errorf("官网更新日志为空")
		}
		// The official version-specific endpoint identifies the release even
		// when a changelog omits its version heading. Reject explicit conflicts.
		heading := regexp.MustCompile(`(?m)^#{1,3}\s+(\d+\.\d+\.\d+(?:-rc\.\d+)?)\s*$`).FindStringSubmatch(log.Content)
		if len(heading) > 1 && heading[1] != v.Name && v.Type == "release" && !strings.Contains(v.Name, "rc") {
			return nil, fmt.Errorf("官网日志与版本%s不一致", v.Name)
		}
		stage := "已发布"
		if strings.Contains(v.Name, "rc") || v.Type != "release" {
			stage = "预发布或归档"
		}
		out = append(out, Event{Key: "version:" + v.Name, Date: date(v.Released), Version: v.Name, Stage: stage, Text: log.Content, URLs: []string{ftbPage, u}})
	}
	return out, nil
}

func (f *Fetcher) curseforge(ctx context.Context) ([]Event, error) {
	b, e := f.get(ctx, cfBase+"/all")
	if e != nil {
		return nil, e
	}
	re := regexp.MustCompile(`/minecraft/modpacks/ftb-evolution-2/files/(\d+)`)
	matches := re.FindAllStringSubmatch(string(b), -1)
	ids := map[string]bool{}
	out := []Event{}
	for _, m := range matches {
		if ids[m[1]] {
			continue
		}
		ids[m[1]] = true
		if len(ids) > 20 {
			break
		}
		u := "https://www.curseforge.com" + m[0]
		b, e = f.get(ctx, u)
		if e != nil {
			return nil, e
		}
		d, e := goquery.NewDocumentFromReader(strings.NewReader(string(b)))
		if e != nil {
			return nil, e
		}
		// Fail closed when CurseForge changes its public changelog markup.
		log := strings.TrimSpace(d.Find(".changelog").Text())
		if log == "" {
			return nil, fmt.Errorf("CurseForge日志正文未识别")
		}
		title := d.Find("h1").First().Text()
		v := regexp.MustCompile(`\b\d+\.\d+\.\d+(?:[-.][A-Za-z0-9]+)*\b`).FindString(title)
		if v == "" {
			v = regexp.MustCompile(`\b\d+\.\d+\.\d+\b`).FindString(log)
		}
		when, ok := d.Find("time[datetime]").First().Attr("datetime")
		if v == "" || !ok || len(when) < 10 {
			return nil, fmt.Errorf("CurseForge版本或日期未核实")
		}
		out = append(out, Event{Key: "version:" + v, Date: when[:10], Version: v, Stage: "已发布", Text: log, URLs: []string{u}})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("CurseForge文件列表未识别")
	}
	return out, nil
}

type ghIssue struct {
	Number  int
	Title   string
	HTMLURL string `json:"html_url"`
	Updated string `json:"updated_at"`
	Labels  []struct{ Name string }
}

func (f *Fetcher) issues(ctx context.Context) ([]Event, error) {
	var items []ghIssue
	u := "https://api.github.com/repos/FTBTeam/FTB-Modpack-Issues/issues?state=all&sort=updated&direction=desc&per_page=100&labels=" + url.QueryEscape("FTB Evolution 2")
	if e := f.json(ctx, u, &items); e != nil {
		return nil, e
	}
	out := []Event{}
	for _, it := range items {
		fixed := false
		for _, l := range it.Labels {
			if l.Name == "state: Fixed in Dev" {
				fixed = true
			}
		}
		if !fixed {
			continue
		}
		cachePath := filepath.Join(f.CacheDir, fmt.Sprintf("issue-%d.json", it.Number))
		var cached struct {
			Updated string
			Event   Event
		}
		if f.CacheDir != "" {
			if b, e := os.ReadFile(cachePath); e == nil && json.Unmarshal(b, &cached) == nil && cached.Updated == it.Updated {
				out = append(out, cached.Event)
				continue
			}
		}
		// Maintainer-controlled labels establish status; player title is context only.
		var comments []struct {
			Body        string
			Association string `json:"author_association"`
			URL         string `json:"html_url"`
			Updated     string `json:"updated_at"`
		}
		u := fmt.Sprintf("https://api.github.com/repos/FTBTeam/FTB-Modpack-Issues/issues/%d/comments?per_page=100", it.Number)
		if e := f.json(ctx, u, &comments); e != nil {
			return nil, e
		}
		text := "官方标签：state: Fixed in Dev。只证明开发中已修复，不证明已经发布。玩家标题仅用于识别问题，不是事实证据：" + it.Title
		urls := []string{it.HTMLURL}
		for _, c := range comments {
			if c.Association == "MEMBER" || c.Association == "OWNER" {
				text += "\n官方维护者回复：" + c.Body
				urls = append(urls, c.URL)
			}
		}
		// updated_at is observation metadata, not a claimed fix/publication date.
		event := Event{Key: fmt.Sprintf("issue:%d", it.Number), Date: "日期未核实", Stage: "开发中（Fixed in Dev，未确认发布）", Text: text, URLs: urls}
		out = append(out, event)
		if f.CacheDir != "" {
			cached.Updated = it.Updated
			cached.Event = event
			if e := Save(cachePath, cached); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}

func (f *Fetcher) repository(ctx context.Context, repo string) ([]Event, error) {
	var releases []struct {
		ID                int
		Tag               string `json:"tag_name"`
		Name, Body        string
		Date              string `json:"published_at"`
		URL               string `json:"html_url"`
		Draft, Prerelease bool
	}
	if e := f.json(ctx, "https://api.github.com/repos/"+repo+"/releases?per_page=30", &releases); e != nil {
		return nil, e
	}
	out := []Event{}
	for _, r := range releases {
		if r.Draft || strings.TrimSpace(r.Body) == "" {
			continue
		}
		stage := "组件已发布（不代表整合包已收录）"
		if r.Prerelease {
			stage = "组件预发布"
		}
		out = append(out, Event{Key: fmt.Sprintf("release:%s:%d", repo, r.ID), Date: r.Date, Version: r.Tag, Stage: stage, Text: r.Name + "\n" + r.Body, URLs: []string{r.URL}})
	}
	var commits []struct {
		SHA    string
		URL    string `json:"html_url"`
		Commit struct {
			Message   string
			Committer struct{ Date string }
		}
	}
	if e := f.json(ctx, "https://api.github.com/repos/"+repo+"/commits?per_page=50", &commits); e != nil {
		return nil, e
	}
	for _, c := range commits {
		out = append(out, Event{Key: "commit:" + repo + ":" + c.SHA, Date: c.Commit.Committer.Date, Stage: "开发中（默认分支提交，非整合包发布）", Text: c.Commit.Message, URLs: []string{c.URL}})
	}
	return out, nil
}

// Collect leaves failed sources uninitialized so their eventual recovery builds
// a baseline, rather than announcing an old backlog as new developments.
func (f *Fetcher) Collect(ctx context.Context, c Config) (map[string][]Event, map[string]string) {
	result := map[string][]Event{}
	errors := map[string]string{}
	add := func(name string, fn func() ([]Event, error)) {
		v, e := fn()
		if e != nil {
			errors[name] = e.Error()
		} else {
			result[name] = v
		}
	}
	if c.FTB {
		add("ftb", func() ([]Event, error) { return f.ftb(ctx) })
	}
	if c.CurseForge {
		add("curseforge", func() ([]Event, error) { return f.curseforge(ctx) })
	}
	if c.GitHub {
		add("github-issues", func() ([]Event, error) { return f.issues(ctx) })
		for _, repo := range c.Repositories {
			add(repo, func() ([]Event, error) { return f.repository(ctx, repo) })
		}
	}
	for _, u := range c.Announcements {
		add(u, func() ([]Event, error) {
			b, e := f.get(ctx, u)
			if e != nil {
				return nil, e
			}
			d, e := goquery.NewDocumentFromReader(strings.NewReader(string(b)))
			if e != nil {
				return nil, e
			}
			article := d.Find("article").First()
			body := plain(article.Text())
			when, _ := article.Find("time[datetime]").Attr("datetime")
			if body == "" || when == "" {
				return nil, fmt.Errorf("官方公告正文或发布日期未识别")
			}
			return []Event{{Key: u, Date: when, Stage: "官方公告（发布状态以正文为准）", Text: body, URLs: []string{u}}}, nil
		})
	}
	return result, errors
}
