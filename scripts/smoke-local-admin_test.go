//go:build ignore

// Run explicitly with smoke-local-admin.go; both files are excluded from builds.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestSmokeArchiveSenderUsesExactMessageMetadata(t *testing.T) {
	for _, raw := range []string{`{"post_type":"message","user_id":9007199254740993}`, `{"post_type":"message","user_id":"9007199254740993"}`} {
		id, e := smokeArchiveSender(raw)
		if e != nil || id != "9007199254740993" {
			t.Fatal(id, e)
		}
	}
	for _, raw := range []string{`{"post_type":"notice","user_id":123}`, `{"post_type":"message_sent","user_id":123}`, `{"post_type":"message","sender":{"user_id":123}}`, `{"message":"user_id=123"}`, `{"post_type":"message","user_id":1.5}`, `{"post_type":"message","user_id":"01"}`, `{"post_type":"message","user_id":9223372036854775808}`, `{"post_type":"message","user_id":true}`, `{"post_type":"message","user_id":123} {}`, `excerpt user_id=123`} {
		if _, e := smokeArchiveSender(raw); e == nil {
			t.Fatal("ambiguous sender accepted", raw)
		}
	}
}

func TestSmokeVisionReplyRequiresRealImageAndDescription(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	imageBody := func(source string) map[string]any {
		return map[string]any{"model": "smoke-vision-model", "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": source}}}}}}
	}
	if reply, err := smokeVisionReply(imageBody("data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes()))); err != nil || reply != strings.Repeat("字", 1500)+"VISION_UNSEEN_TAIL" {
		t.Fatal(err, reply)
	}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := smokeVisionReply(imageBody("data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegData.Bytes()))); err != nil {
		t.Fatal("actual preprocessing JPEG rejected", err)
	}
	if _, err := smokeVisionReply(imageBody("data:image/png;base64," + base64.StdEncoding.EncodeToString(jpegData.Bytes()))); err == nil {
		t.Fatal("incorrect image MIME accepted")
	}
	for _, source := range []string{"https://synthetic.invalid/image", "data:image/png;base64,!", "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not PNG"))} {
		if _, err := smokeVisionReply(imageBody(source)); err == nil {
			t.Fatal("invalid vision input accepted")
		}
	}
	event := func(description string, invalid bool) map[string]any {
		bad := map[string]any{"file": "base64://!"}
		if invalid {
			bad["__agent_desc__"] = "fabricated"
		}
		return map[string]any{"post_type": "message", "message": []any{map[string]any{"type": "text", "data": map[string]any{"text": "VISION_IMAGE_CHECK"}}, map[string]any{"type": "image", "data": map[string]any{"file": "https://synthetic.invalid/agent-image-ok", "__agent_desc__": description, "__agent_desc_truncated__": "true"}}, map[string]any{"type": "image", "data": bad}, map[string]any{"type": "image", "data": map[string]any{"file": "file:///synthetic.png"}}, map[string]any{"type": "image", "data": map[string]any{"file": "https://synthetic.invalid/agent-image-large"}}}}
	}
	if _, err := smokeVisionReply(knowledgeTestBody("smoke-vision-agent", event(strings.Repeat("字", 1333), false))); err != nil {
		t.Fatal(err)
	}
	for _, record := range []map[string]any{event("missing", false), event(strings.Repeat("字", 1333), true), {"post_type": "message", "message": []any{}}} {
		if _, err := smokeVisionReply(knowledgeTestBody("smoke-vision-agent", record)); err == nil {
			t.Fatal("fabricated or absent description accepted")
		}
	}
	if _, err := smokeVisionReply(knowledgeTestBody("smoke-vision-agent", event(strings.Repeat("字", 1333), false), event("missing", false))); err == nil {
		t.Fatal("old success masked latest failure")
	}
}

func TestSmokeDescriptionRecoveryRequiresActualCurrentCaptions(t *testing.T) {
	event := func(recovered bool, broken string) map[string]any {
		segments := []any{map[string]any{"type": "text", "data": map[string]any{"text": "VISION_DESCRIPTION_CHECK"}}}
		for _, name := range []string{"a", "b", "c"} {
			data := map[string]any{"file": "https://synthetic.invalid/description-image-" + name}
			if recovered || name == "c" {
				data["__agent_desc__"] = "RECOVERED_IMAGE_DESCRIPTION"
			}
			if broken == name {
				data["__agent_desc__"] = "fabricated"
			}
			if broken == "flag" {
				data["__agent_desc_truncated__"] = "true"
			}
			segments = append(segments, map[string]any{"type": "image", "data": data})
		}
		return map[string]any{"post_type": "message", "message": segments}
	}
	for _, round := range []int{1, 2} {
		if _, err := smokeDescriptionReply(knowledgeTestBody("smoke-description-agent", event(round == 2, "")), round); err != nil {
			t.Fatal(err)
		}
	}
	for _, broken := range []string{"a", "b", "c", "flag"} {
		if _, err := smokeDescriptionReply(knowledgeTestBody("smoke-description-agent", event(false, broken)), 1); err == nil {
			t.Fatal("invalid first-round captions accepted", broken)
		}
	}
	if _, err := smokeDescriptionReply(knowledgeTestBody("smoke-description-agent", event(true, ""), event(false, "")), 2); err == nil {
		t.Fatal("old recovery masked latest missing descriptions")
	}
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	body := map[string]any{"model": "smoke-description-vision", "messages": []any{map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())}}}}}}
	for _, round := range []int{1, 2, 3} {
		reply, err := smokeDescriptionReply(body, round)
		if err != nil {
			t.Fatal(err)
		}
		if (round == 1 && strings.TrimSpace(reply) != "") || (round == 2 && len(reply) <= 256<<10) || (round == 3 && reply != "RECOVERED_IMAGE_DESCRIPTION") {
			t.Fatal("incorrect fixture response", round)
		}
	}
	if _, err := smokeDescriptionReply(body, 4); err == nil {
		t.Fatal("unexpected request after recovery accepted")
	}
}

func TestSmokeStructureFixturesStayBelowOtherLimits(t *testing.T) {
	for _, kind := range []string{"structure-depth", "structure-values"} {
		raw, e := smokeActionOutputReply(kind)
		if e != nil || len(raw) >= 1<<20 {
			t.Fatal("invalid structure fixture", kind, e)
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		count := 0
		for decoder.More() {
			var action map[string]any
			if e = decoder.Decode(&action); e != nil {
				t.Fatal("structure fixture is not valid JSON", e)
			}
			if count == 0 && action["action"] != "save_memory" {
				t.Fatal("missing memory prefix")
			}
			count++
		}
		if count >= 32 || count < 2 {
			t.Fatal("structure case accidentally exceeded action limit", count)
		}
		if kind == "structure-depth" && strings.Count(raw, "[") != 63 {
			t.Fatal("wrong depth case")
		}
		if kind == "structure-values" && (strings.Count(raw, "null") != 10000 || count != 3) {
			t.Fatal("wrong cumulative value case")
		}
	}
	good, e := smokeActionOutputReply("ok")
	if e != nil || strings.Count(good, "[") != 64 || !strings.Contains(good, "RECOVERY_OK") {
		t.Fatal("normal recovery lost exact-depth case", e)
	}
}

func TestSmokeActionOutputLimits(t *testing.T) {
	batch, err := smokeActionOutputReply("batch")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(batch))
	count := 0
	first := ""
	for dec.More() {
		var r map[string]any
		if err = dec.Decode(&r); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			first, _ = r["action"].(string)
		}
		count++
	}
	if count != 33 || first != "save_memory" {
		t.Fatal("batch fixture did not test partial saving", count)
	}
	text, _ := smokeActionOutputReply("text")
	body, _ := smokeActionOutputReply("http")
	if len(text) <= 1<<20 || len(text) >= 2<<20 || len(body) <= 2<<20 {
		t.Fatal("output fixture boundaries incorrect")
	}
	stamp := "2026-10-15T00:00:00Z"
	base := map[string]any{"persisted": true, "status": "failed", "phase": "finished", "error": "model_output_limit", "startedAt": stamp, "endedAt": stamp, "phaseStartedAt": stamp, "activity": map[string]any{"available": true, "modelPreparations": float64(1), "fallbackPreparations": float64(0), "localActions": float64(0), "platformActions": float64(0)}, "results": map[string]any{"available": true, "ok": float64(0), "rejected": float64(0), "unknown": float64(0), "localErrors": float64(0)}, "queries": map[string]any{"available": true, "limit": float64(3), "used": float64(0), "successful": float64(0), "empty": float64(0), "errors": float64(0), "denied": float64(0)}}
	if !smokeOutputLimitTask(base, true) {
		t.Fatal("valid output failure rejected")
	}
	for _, bad := range []map[string]any{{"persisted": false}, {"status": "completed"}, {"error": "execution_error"}, {"activity": map[string]any{"available": true, "modelPreparations": float64(1), "fallbackPreparations": float64(1), "localActions": float64(0), "platformActions": float64(0)}}} {
		v := map[string]any{}
		for k, x := range base {
			v[k] = x
		}
		for k, x := range bad {
			v[k] = x
		}
		if smokeOutputLimitTask(v, true) {
			t.Fatal("invalid output failure accepted", v)
		}
	}
	base["error"] = "model_output_invalid"
	if !smokeOutputTask(base, true, "model_output_invalid") || smokeOutputLimitTask(base, true) {
		t.Fatal("syntax failure confused with capacity failure")
	}
}

func TestSmokeMalformedActionFixtures(t *testing.T) {
	for _, kind := range []string{"syntax-closing", "syntax-truncated", "syntax-null"} {
		output, err := smokeActionOutputReply(kind)
		if err != nil || !strings.Contains(output, "LIMIT_BATCH_MUST_NOT_SAVE") || !strings.Contains(output, "MALFORMED_MUST_NOT_SEND") {
			t.Fatal("missing rejected batch prefix", kind, err)
		}
		dec := json.NewDecoder(strings.NewReader(output))
		for i := 0; i < 2; i++ {
			var value map[string]any
			if err := dec.Decode(&value); err != nil || value["action"] == nil {
				t.Fatal("missing valid prefix", kind, err)
			}
		}
		var tail map[string]any
		if err := dec.Decode(&tail); err == nil && tail != nil {
			t.Fatal("fixture accidentally valid", kind)
		}
	}
}

func TestSmokeDuplicateActionFixturesAndRecovery(t *testing.T) {
	for _, kind := range []string{"syntax-duplicate-action", "syntax-duplicate-params"} {
		output, err := smokeActionOutputReply(kind)
		if err != nil || !strings.Contains(output, "LIMIT_BATCH_MUST_NOT_SAVE") || !strings.Contains(output, "MALFORMED_MUST_NOT_SEND") {
			t.Fatal("missing prefix in ambiguous fixture", kind, err)
		}
		dec := json.NewDecoder(strings.NewReader(output))
		for i := 0; i < 2; i++ {
			var prefix json.RawMessage
			if err := dec.Decode(&prefix); err != nil {
				t.Fatal(err)
			}
		}
		var tail apiRequest
		if err := dec.Decode(&tail); err != nil {
			t.Fatal("fixture is malformed instead of ambiguous", err)
		}
		if (kind == "syntax-duplicate-action" && tail.Action != "end_action") || (kind == "syntax-duplicate-params" && tail.Params.GroupID != 609) {
			t.Fatal("fixture no longer demonstrates field overwrite", kind, tail)
		}
	}
	output, err := smokeActionOutputReply("ok")
	var req apiRequest
	if err != nil || json.NewDecoder(strings.NewReader(output)).Decode(&req) != nil {
		t.Fatal("normal recovery fixture invalid", err)
	}
	text, err := smokeMessageText(req.Params.Message)
	var segments []json.RawMessage
	if err != nil || text != "OUTPUT_LIMIT_RECOVERY_OK" || json.Unmarshal(req.Params.Message, &segments) != nil || len(segments) != 2 {
		t.Fatal("normal recovery no longer tests sibling message keys", text, err)
	}
}

func TestSmokeModelRequestFailureFixtures(t *testing.T) {
	for _, kind := range []string{"request-http", "request-protocol", "request-empty"} {
		status, text, ok := smokeModelRequestFailure(kind)
		want := 503
		if kind != "request-http" {
			want = 200
		}
		wantBody := smokeModelFailureText
		if kind == "request-empty" {
			wantBody = ""
		}
		if !ok || status != want || text != wantBody || json.Valid([]byte(text)) || len(text) > 1024 {
			t.Fatal("incorrect model failure fixture", kind, status)
		}
	}
	if _, _, ok := smokeModelRequestFailure("ok"); ok {
		t.Fatal("normal recovery replaced by request failure")
	}
}

func TestSmokeModelOutputShapes(t *testing.T) {
	for _, kind := range []string{"request-missing", "request-unsupported"} {
		status, text, ok := smokeModelRequestFailure(kind)
		if !ok || status != 200 || !json.Valid([]byte(text)) || !strings.Contains(text, smokeModelFailureText) {
			t.Fatal("output shape fixture is not a valid protocol object", kind)
		}
		var envelope map[string]any
		json.Unmarshal([]byte(text), &envelope)
		if kind == "request-missing" && envelope["choices"] != nil {
			t.Fatal("missing output fixture has choices")
		}
		if kind == "request-unsupported" && (!strings.Contains(text, "private-unknown-type") || !strings.Contains(text, "MUST_NOT_SAVE")) {
			t.Fatal("unsupported output lost valid prefix")
		}
	}
	reply, err := smokeActionOutputReply("ok")
	if err != nil {
		t.Fatal(err)
	}
	envelope := smokeModelReplyEnvelope("smoke-output-limit-ok", reply)
	message := envelope["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	parts, ok := message["content"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatal("model recovery is not multipart")
	}
	var joined strings.Builder
	for _, raw := range parts {
		part := raw.(map[string]any)
		if part["type"] != "text" {
			t.Fatal("non-text normal fixture")
		}
		joined.WriteString(part["text"].(string))
	}
	if joined.String() != reply {
		t.Fatal("multipart fixture altered JSON")
	}
	for _, name := range []string{"smoke-vision-model", "smoke-description-vision"} {
		text := "图片包含🙂和中文"
		message := smokeModelReplyEnvelope(name, text)["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
		parts := message["content"].([]any)
		encoded, err := json.Marshal(message)
		if err != nil || !json.Valid(encoded) || len(parts) != 2 || parts[0].(map[string]any)["text"].(string)+parts[1].(map[string]any)["text"].(string) != text {
			t.Fatal("vision multipart fixture altered text", name)
		}
	}
	ordinary := smokeModelReplyEnvelope("smoke-prepare-vision", "description")["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if ordinary["content"] != "description" {
		t.Fatal("other model formats changed")
	}
}

func TestSmokePreparationStopSummary(t *testing.T) {
	stamp := "2026-10-15T00:00:00Z"
	base := map[string]any{"persisted": true, "status": "cancelled", "phase": "finished", "startedAt": stamp, "endedAt": stamp, "phaseStartedAt": stamp, "cancelRequestedAt": stamp, "activity": map[string]any{"available": true, "modelPreparations": float64(1), "fallbackPreparations": float64(0), "localActions": float64(0), "platformActions": float64(0)}, "results": map[string]any{"available": true, "ok": float64(0), "rejected": float64(0), "unknown": float64(0), "localErrors": float64(0)}, "queries": map[string]any{"available": true, "limit": float64(3), "used": float64(0), "successful": float64(0), "empty": float64(0), "errors": float64(0), "denied": float64(0)}}
	if !smokePreparationStoppedTask(base) {
		t.Fatal("valid preparation stop rejected")
	}
	for _, bad := range []map[string]any{{"status": "completed"}, {"persisted": false}, {"cancelRequestedAt": ""}, {"activity": map[string]any{"available": true, "modelPreparations": float64(1), "fallbackPreparations": float64(1), "localActions": float64(0), "platformActions": float64(0)}}, {"results": map[string]any{"available": true, "ok": float64(1), "rejected": float64(0), "unknown": float64(0), "localErrors": float64(0)}}} {
		v := map[string]any{}
		for k, x := range base {
			v[k] = x
		}
		for k, x := range bad {
			v[k] = x
		}
		if smokePreparationStoppedTask(v) {
			t.Fatal("invalid preparation stop accepted", v)
		}
	}
}

func TestSmokeTaskObservationValid(t *testing.T) {
	task := map[string]any{"startedAt": "2026-10-15T00:00:00Z", "phaseStartedAt": "2026-10-15T00:00:01.123456789Z", "endedAt": "2026-10-15T00:00:02Z"}
	if !smokeTaskObservationValid(map[string]any{"observedAt": "2026-10-15T00:00:03Z"}, task) {
		t.Fatal("valid observation rejected")
	}
	for _, stamp := range []any{nil, 123, "bad", "2026-10-15T00:00:00Z", "2026-10-15T08:00:03+08:00"} {
		if smokeTaskObservationValid(map[string]any{"observedAt": stamp}, task) {
			t.Fatal("invalid observation accepted", stamp)
		}
	}
}

func TestSmokeTaskTimingValid(t *testing.T) {
	stamp := "2026-10-14T00:00:00.123456789Z"
	base := map[string]any{"persisted": true, "startedAt": stamp, "phaseStartedAt": stamp, "phase": "model_wait", "status": "running"}
	if !smokeTaskTimingValid(base) {
		t.Fatal("valid running timing rejected")
	}
	for _, invalid := range []map[string]any{{"persisted": false}, {"phaseStartedAt": nil}, {"phaseStartedAt": 123}, {"phaseStartedAt": "2026-02-30T00:00:00Z"}, {"phaseStartedAt": "2026-10-14T08:00:00+08:00"}, {"status": "cancel_requested"}, {"cancelRequestedAt": "bad"}, {"phase": "finished", "endedAt": "2026-10-14T00:00:01Z"}} {
		v := map[string]any{}
		for k, x := range base {
			v[k] = x
		}
		for k, x := range invalid {
			v[k] = x
		}
		if smokeTaskTimingValid(v) {
			t.Fatal("invalid timing accepted", v)
		}
	}
	base["cancelRequestedAt"] = stamp
	base["status"] = "cancelled"
	base["phase"] = "finished"
	base["endedAt"] = stamp
	if !smokeTaskTimingValid(base) {
		t.Fatal("valid terminal timing rejected")
	}
}

func knowledgeTestBody(model string, records ...any) map[string]any {
	lines := []string{}
	for _, record := range records {
		raw, _ := json.Marshal(record)
		lines = append(lines, string(raw))
	}
	return map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": strings.Join(lines, "\n")}}}
}

func knowledgeTestResponse(field string, rows []any) map[string]any {
	return map[string]any{"status": "ok", "data": map[string]any{field: rows}}
}

func knowledgeTestEvent(text string) map[string]any {
	return map[string]any{"post_type": "message", "message": []any{map[string]any{"type": "text", "data": map[string]any{"text": text}}}}
}

func TestSmokeHistoryContextRequiresActualAnchorAndBoundedNeighbors(t *testing.T) {
	for _, kind := range []string{"valid", "missing-neighbor", "wrong-anchor", "wrong-radius", "wrong-source", "tool", "duplicate", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			center := map[string]any{"seq": "9007199254740993", "kind": "event", "text": "center"}
			neighbor := map[string]any{"seq": "9007199254740994", "kind": "event", "text": "neighbor"}
			previous := knowledgeTestResponse("sources", []any{center})
			request := map[string]any{"action": "read_history", "params": map[string]any{"seq": center["seq"], "context": 1}}
			response := knowledgeTestResponse("sources", []any{neighbor, center})
			data := response["data"].(map[string]any)
			data["anchorSeq"] = center["seq"]
			data["context"] = 1
			data["searchMore"] = false
			switch kind {
			case "missing-neighbor":
				data["sources"] = []any{center}
			case "wrong-anchor":
				data["anchorSeq"] = "1"
			case "wrong-radius":
				data["context"] = 2
			case "wrong-source":
				previous = knowledgeTestResponse("sources", []any{neighbor})
			case "tool":
				neighbor["kind"] = "response"
			case "duplicate":
				neighbor["seq"] = center["seq"]
			case "oversized":
				neighbor["text"] = strings.Repeat("x", 2001)
			}
			anchor, err := smokeHistoryContextAnchor(knowledgeTestBody("fixture", previous, request, response))
			if kind == "valid" {
				if err != nil || anchor["seq"] != "9007199254740993" {
					t.Fatal(anchor, err)
				}
			} else if err == nil {
				t.Fatal("bad context accepted", kind)
			}
		})
	}
}

func TestSmokeSaveMetadataUsesContextSourceDelta(t *testing.T) {
	before := map[string]any{"persisted": true, "context": map[string]any{"available": true, "clipped": false, "memorySources": float64(3)}}
	after := map[string]any{"persisted": true, "context": map[string]any{"available": true, "clipped": false, "memorySources": float64(4)}}
	if !smokeMemorySourceDelta(before, after) {
		t.Fatal("prior response sources ignored")
	}
	for _, bad := range []any{float64(1), float64(3), float64(6), "4", nil} {
		after["context"].(map[string]any)["memorySources"] = bad
		if smokeMemorySourceDelta(before, after) {
			t.Fatal("wrong source delta accepted", bad)
		}
	}
	after["context"].(map[string]any)["memorySources"] = float64(4)
	after["context"].(map[string]any)["clipped"] = true
	if smokeMemorySourceDelta(before, after) {
		t.Fatal("clipped context compared as complete")
	}
}

func TestSmokeSavedReferenceDrivesExactRead(t *testing.T) {
	model := "smoke-knowledge-save"
	save := map[string]any{"action": "save_memory", "params": map[string]any{"text": "KNOWLEDGE_SAVED_FROM_RESULT"}}
	ref := map[string]any{"id": "9007199254740993", "revision": "9223372036854775807", "source": "agent", "sourceSeq": "23"}
	response := func(created bool, reference map[string]any) map[string]any {
		return map[string]any{"status": "ok", "data": map[string]any{"mode": "managed", "created": created, "memory": reference}}
	}
	first, reused := response(true, ref), response(false, ref)
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model), 1); e != nil || strings.Count(reply, `"action":"save_memory"`) != 2 {
		t.Fatal(reply, e)
	}
	body := knowledgeTestBody(model, save, first, save, reused)
	reply, e := smokeKnowledgeReply(body, 2)
	if e != nil || !strings.Contains(reply, `"id":"9007199254740993"`) || !strings.Contains(reply, `"revision":"9223372036854775807"`) {
		t.Fatal("actual large saved reference not used", reply, e)
	}
	item := map[string]any{"id": ref["id"], "revision": ref["revision"], "source": ref["source"], "sourceSeq": ref["sourceSeq"], "text": "KNOWLEDGE_SAVED_FROM_RESULT", "truncated": false}
	read := map[string]any{"action": "read_memory", "params": map[string]any{"id": ref["id"], "revision": ref["revision"]}}
	if reply, e = smokeKnowledgeReply(knowledgeTestBody(model, save, first, save, reused, read, knowledgeTestResponse("memories", []any{item})), 3); e != nil || !strings.Contains(reply, "KNOWLEDGE_SAVE_OK") {
		t.Fatal(reply, e)
	}
	for _, bad := range []any{nil, float64(9007199254740993), "01", "9223372036854775808"} {
		invalid := map[string]any{"id": bad, "revision": ref["revision"], "source": ref["source"], "sourceSeq": ref["sourceSeq"]}
		if _, e = smokeKnowledgeReply(knowledgeTestBody(model, save, first, save, response(false, invalid)), 2); e == nil {
			t.Fatal("invalid saved ID accepted", bad)
		}
	}
	for _, records := range [][]any{{save, reused, save, reused}, {save, first, save, first}, {save, first, save, reused, map[string]any{"status": "error", "data": nil}}} {
		if _, e = smokeKnowledgeReply(knowledgeTestBody(model, records...), 2); e == nil {
			t.Fatal("missing creation/reuse or stale success accepted")
		}
	}
	wrong := map[string]any{"id": "9007199254740994", "revision": ref["revision"], "source": ref["source"], "sourceSeq": ref["sourceSeq"], "text": "KNOWLEDGE_SAVED_FROM_RESULT", "truncated": false}
	if _, e = smokeKnowledgeReply(knowledgeTestBody(model, save, first, save, reused, read, knowledgeTestResponse("memories", []any{wrong})), 3); e == nil {
		t.Fatal("read of another memory accepted")
	}
}

func TestSmokeTaskQuerySummaryRejectsUnknownOrFailedCoverage(t *testing.T) {
	q := map[string]any{"available": true, "limit": float64(3), "used": float64(1), "successful": float64(1), "empty": float64(1), "errors": float64(0), "denied": float64(0)}
	task := map[string]any{"persisted": true, "queries": q}
	if !smokeTaskQueriesMatch(task, 1, 1, 1) {
		t.Fatal("valid quota/coverage rejected")
	}
	for _, tc := range []struct {
		field string
		bad   any
	}{{"available", false}, {"limit", float64(4)}, {"used", "1"}, {"successful", float64(0)}, {"empty", float64(0)}, {"errors", float64(1)}, {"denied", float64(1)}} {
		old := q[tc.field]
		q[tc.field] = tc.bad
		if smokeTaskQueriesMatch(task, 1, 1, 1) {
			t.Fatal("bad query summary accepted", tc)
		}
		q[tc.field] = old
	}
	task["persisted"] = false
	if smokeTaskQueriesMatch(task, 1, 1, 1) {
		t.Fatal("unwritten query coverage accepted")
	}
}

func TestSmokeTaskResultsRequireDurableSettledCounts(t *testing.T) {
	task := map[string]any{"persisted": true, "results": map[string]any{"available": true, "ok": float64(2), "rejected": float64(1), "unknown": float64(3), "localErrors": float64(4)}}
	if !smokeTaskResultsMatch(task, 2, 1, 3, 4) {
		t.Fatal("valid counts rejected")
	}
	if smokeTaskResultsMatch(task, 3, 1, 3, 4) {
		t.Fatal("duplicate result accepted")
	}
	task["persisted"] = false
	if smokeTaskResultsMatch(task, 2, 1, 3, 4) {
		t.Fatal("uncertain counts accepted")
	}
	task["persisted"] = true
	for _, invalid := range []any{nil, map[string]any{"available": false}, map[string]any{"available": true, "ok": "2", "rejected": float64(1), "unknown": float64(3), "localErrors": float64(4)}} {
		task["results"] = invalid
		if smokeTaskResultsMatch(task, 2, 1, 3, 4) {
			t.Fatal("missing or invalid counts accepted")
		}
	}
}

func TestSmokeMemoryRequiresExactTrigger(t *testing.T) {
	const seq = "9007199254740993"
	memory := map[string]any{"source": "agent", "sourceSeq": seq}
	task := map[string]any{"sourceSeq": seq}
	entry := map[string]any{"seq": seq, "kind": "event", "text": "ROLE_AGENT_CHECK"}
	if !smokeMemoryMatchesTrigger(memory, task, entry) {
		t.Fatal("exact large source rejected")
	}
	for _, tc := range []struct {
		field string
		value any
	}{{"seq", "9007199254740994"}, {"seq", float64(9007199254740993)}, {"kind", "response"}, {"text", "ROLE_AGENT_CROSS_INFO"}} {
		copy := map[string]any{"seq": seq, "kind": "event", "text": "ROLE_AGENT_CHECK"}
		copy[tc.field] = tc.value
		if smokeMemoryMatchesTrigger(memory, task, copy) {
			t.Fatal("wrong event accepted", tc)
		}
	}
	for _, bad := range []any{"0", "01", "9007199254740994", nil, float64(9007199254740993)} {
		task["sourceSeq"] = bad
		if smokeMemoryMatchesTrigger(memory, task, entry) {
			t.Fatal("wrong task source accepted", bad)
		}
	}
	task["sourceSeq"] = seq
	memory["source"] = "admin"
	if smokeMemoryMatchesTrigger(memory, task, entry) {
		t.Fatal("admin source accepted")
	}
}

func TestSmokeKnowledgeMemoryUsesActualResponses(t *testing.T) {
	model := "smoke-knowledge-memory"
	const from, until = "2000-01-01T00:00:00.000000001Z", "2100-01-01T00:00:00Z"
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model), 1); e != nil || !strings.Contains(reply, `"action":"search_memory"`) || !strings.Contains(reply, `"source":"admin"`) || !strings.Contains(reply, `"from":"2000-01-01T08:00:00.000000001+08:00"`) {
		t.Fatalf("initial query: %s %v", reply, e)
	}
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestEvent("KNOWLEDGE_TAIL_ONLY_READ")), 1); e == nil {
		t.Fatal("initial target leak accepted")
	}
	item := map[string]any{"id": "9223372036854775807", "revision": "13", "source": "admin", "sourceSeq": "0", "text": "KNOWLEDGE_MANUAL_TARGET KNOWLEDGE_NEWER_DISTRACTOR", "truncated": false}
	response := knowledgeTestResponse("memories", []any{item})
	response["data"].(map[string]any)["searchMore"] = true
	response["data"].(map[string]any)["source"] = "admin"
	response["data"].(map[string]any)["from"], response["data"].(map[string]any)["until"] = from, until
	response["data"].(map[string]any)["nextBefore"] = "9007199254740994"
	body := knowledgeTestBody(model, response)
	if reply, e := smokeKnowledgeReply(body, 2); e != nil || !strings.Contains(reply, `"before":"9007199254740994"`) || !strings.Contains(reply, `"action":"search_memory"`) || !strings.Contains(reply, `"source":"admin"`) || !strings.Contains(reply, `"from":"`+from+`"`) || !strings.Contains(reply, `"until":"`+until+`"`) {
		t.Fatalf("older query must preserve returned string ID: %s %v", reply, e)
	}
	for _, bad := range []any{nil, "0", "9223372036854775808", 9007199254740994} {
		value := knowledgeTestResponse("memories", []any{item})
		value["data"].(map[string]any)["searchMore"] = true
		value["data"].(map[string]any)["nextBefore"] = bad
		value["data"].(map[string]any)["source"] = "admin"
		value["data"].(map[string]any)["from"], value["data"].(map[string]any)["until"] = from, until
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, value), 2); e == nil {
			t.Fatal("invalid memory continuation accepted", bad)
		}
	}
	request := map[string]any{"action": "search_memory", "params": map[string]any{"before": "9007199254740994", "source": "admin", "from": from, "until": until}}
	searchResponse := func() map[string]any {
		value := knowledgeTestResponse("memories", []any{item})
		value["data"].(map[string]any)["source"] = "admin"
		value["data"].(map[string]any)["from"], value["data"].(map[string]any)["until"] = from, until
		return value
	}
	item["id"], item["text"], item["truncated"] = "9007199254740993", "KNOWLEDGE_MANUAL_TARGET", true
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model, request, searchResponse()), 3); e != nil || !strings.Contains(reply, `"id":"9007199254740993"`) || !strings.Contains(reply, `"revision":"13"`) {
		t.Fatalf("read must preserve returned older ID/version: %s %v", reply, e)
	}
	item["id"] = "9007199254740994"
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, request, searchResponse()), 3); e == nil {
		t.Fatal("equal memory boundary accepted")
	}
	item["id"] = "9007199254740993"
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, searchResponse()), 3); e == nil {
		t.Fatal("older result without actual boundary accepted")
	}
	item["text"], item["truncated"] = "KNOWLEDGE_MANUAL_TARGET KNOWLEDGE_TAIL_ONLY_READ", false
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestResponse("memories", []any{item})), 4); e != nil || !strings.Contains(reply, "KNOWLEDGE_MEMORY_OK") {
		t.Fatalf("final text: %s %v", reply, e)
	}
	item["text"] = "KNOWLEDGE_MANUAL_TARGET"
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestResponse("memories", []any{item})), 4); e == nil {
		t.Fatal("missing tail accepted")
	}
	item["id"] = float64(12)
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestResponse("memories", []any{item})), 2); e == nil {
		t.Fatal("numeric memory identity accepted")
	}
	// An earlier good response cannot hide the latest failure/missing result.
	item["id"], item["text"], item["truncated"] = "9223372036854775807", "KNOWLEDGE_MANUAL_TARGET KNOWLEDGE_NEWER_DISTRACTOR", false
	for _, bad := range []map[string]any{
		{"status": "error", "data": nil},
		knowledgeTestResponse("memories", []any{}),
		knowledgeTestResponse("sources", []any{}),
	} {
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestResponse("memories", []any{item}), bad), 2); e == nil {
			t.Fatal("latest response failure hidden by earlier result")
		}
	}
}

func TestSmokeKnowledgeHistoryBoundaryAndStaleRead(t *testing.T) {
	model := "smoke-knowledge-history"
	event := knowledgeTestEvent("KNOWLEDGE_HISTORY_CHECK BEFORE=9223372036854775807 FROM=2026-10-15T08:00:00+08:00 UNTIL=2026-10-15T00:00:00.000000001Z SENDER=9007199254740993")
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model, event), 1); e != nil || !strings.Contains(reply, `"before":"9223372036854775807"`) || !strings.Contains(reply, `"sender":"9007199254740993"`) {
		t.Fatalf("history boundary: %s %v", reply, e)
	}
	item := map[string]any{"seq": "9223372036854775806", "kind": "event", "text": `{"post_type":"message","user_id":9007199254740993,"message":"ROLE_AGENT_CHECK"}`, "time": "2026-10-15T00:00:00Z"}
	response := func() map[string]any {
		value := knowledgeTestResponse("sources", []any{item})
		data := value["data"].(map[string]any)
		data["from"] = "2026-10-15T00:00:00Z"
		data["until"] = "2026-10-15T00:00:00.000000001Z"
		data["searchMore"] = true
		data["sender"] = "9007199254740993"
		data["eventType"] = "message"
		data["nextBefore"] = "9223372036854775805"
		return value
	}
	body := knowledgeTestBody(model, event, response())
	if reply, e := smokeKnowledgeReply(body, 2); e != nil || !strings.Contains(reply, `"before":"9223372036854775805"`) || !strings.Contains(reply, `"action":"search_history"`) || strings.Contains(reply, `"keyword"`) {
		t.Fatalf("continue from receipt: %s %v", reply, e)
	}
	for _, bad := range []any{"0", "9223372036854775807", float64(9007199254740992), nil} {
		value := response()
		value["data"].(map[string]any)["nextBefore"] = bad
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, value), 2); e == nil {
			t.Fatal("invalid continuation accepted", bad)
		}
	}
	for _, bad := range []any{"123", float64(9007199254740992), nil} {
		value := response()
		value["data"].(map[string]any)["sender"] = bad
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, value), 2); e == nil {
			t.Fatal("unconfirmed sender accepted", bad)
		}
	}
	for _, bad := range []any{"notice", "request", "", nil} {
		value := response()
		value["data"].(map[string]any)["eventType"] = bad
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, value), 2); e == nil {
			t.Fatal("unconfirmed event type accepted", bad)
		}
	}
	originalText := item["text"]
	item["text"] = `{"post_type":"message","user_id":123,"message":"ROLE_AGENT_CHECK mentions 9007199254740993"}`
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, response()), 2); e == nil {
		t.Fatal("mentioned account mistaken for sender")
	}
	item["text"] = originalText
	item["seq"] = "9007199254740993"
	item["text"] = `{"post_type":"message","user_id":9007199254740993,"message":"different recent message without the original search phrase"}`
	continued := map[string]any{"action": "search_history", "params": map[string]any{"before": "9223372036854775805", "sender": "9007199254740993", "eventType": "message"}}
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model, event, continued, response()), 3); e != nil || !strings.Contains(reply, `"seq":"9007199254740993"`) {
		t.Fatal("continued source not read", reply, e)
	}
	contextRows := []any{map[string]any{"seq": "9007199254740994", "kind": "event", "text": "neighbor newer"}, item, map[string]any{"seq": "9007199254740992", "kind": "event", "text": "neighbor older"}}
	contextReply := knowledgeTestResponse("sources", contextRows)
	contextReply["data"].(map[string]any)["anchorSeq"] = item["seq"]
	contextReply["data"].(map[string]any)["context"] = 1
	contextReply["data"].(map[string]any)["searchMore"] = false
	readRequest := map[string]any{"action": "read_history", "params": map[string]any{"seq": item["seq"], "context": 1}}
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model, event, continued, response(), readRequest, contextReply), 4); e != nil || !strings.Contains(reply, "KNOWLEDGE_HISTORY_OK") {
		t.Fatal("continued source did not finish", reply, e)
	}
	item["seq"] = "9223372036854775806"
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, knowledgeTestResponse("sources", []any{item})), 2); e == nil {
		t.Fatal("missing applied range accepted")
	}
	for _, stamp := range []string{"2026-10-14T23:59:59.999999999Z", "2026-10-15T00:00:00.000000001Z", "invalid"} {
		item["time"] = stamp
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, response()), 2); e == nil {
			t.Fatal("out-of-range history accepted", stamp)
		}
	}
	item["time"] = "2026-10-15T00:00:00Z"
	item["seq"] = "9223372036854775807"
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, response()), 2); e == nil {
		t.Fatal("exclusive history boundary accepted equal sequence")
	}
	item["seq"], item["kind"] = "1", "response"
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, event, response()), 2); e == nil {
		t.Fatal("non-event source accepted")
	}
	for _, id := range []string{"0", "01", "-1", "9223372036854775808", "1.0"} {
		if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestEvent("KNOWLEDGE_HISTORY_CHECK BEFORE="+id)), 1); e == nil {
			t.Fatalf("invalid boundary accepted: %s", id)
		}
	}
	model = "smoke-knowledge-stale"
	if reply, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestEvent("KNOWLEDGE_STALE_CHECK ID=23 REV=1")), 1); e != nil || !strings.Contains(reply, `"action":"read_memory"`) {
		t.Fatalf("stale read request: %s %v", reply, e)
	}
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestResponse("memories", []any{})), 2); e != nil {
		t.Fatal(e)
	}
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, knowledgeTestResponse("memories", []any{item})), 2); e == nil {
		t.Fatal("stale result unexpectedly returned text")
	}
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model, map[string]any{"status": "error", "data": nil}), 2); e == nil {
		t.Fatal("failed read counted as proven stale result")
	}
	if _, e := smokeKnowledgeReply(knowledgeTestBody(model), 4); e == nil {
		t.Fatal("unplanned model round accepted")
	}
}
