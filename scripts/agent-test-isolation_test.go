package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportBoundaryAndChangedSourceRefusal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "123.sqlite")
	db, e := sql.Open("sqlite3", path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for _, q := range []string{`CREATE TABLE history(seq INTEGER PRIMARY KEY,conversation INTEGER,kind TEXT,created_at TEXT,payload TEXT)`, `CREATE TABLE agent_memories(id INTEGER PRIMARY KEY,conversation INTEGER,revision INTEGER,text TEXT,updated_at TEXT)`, `INSERT INTO history VALUES(1,10,'event','2026-10-28T17:53:08Z','{"post_type":"message","message":"past"}'),(2,10,'event','2026-10-28T17:53:09Z','{"post_type":"message","message":"future"}')`, `INSERT INTO agent_memories VALUES(1,10,1,'past memory','2026-10-28T17:53:08Z'),(2,10,1,'future memory','2026-10-28T17:53:09Z')`} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	oldDB, oldOut, oldBefore, oldApply, oldLegacy := *dbFile, *output, *cutoff, *applyPlan, *legacyDir
	defer func() {
		*dbFile, *output, *cutoff, *applyPlan, *legacyDir = oldDB, oldOut, oldBefore, oldApply, oldLegacy
	}()
	*dbFile = path
	*output = filepath.Join(root, "export")
	*cutoff = "2026-10-28T17:53:08Z"
	*applyPlan = ""
	*legacyDir = ""
	if e = run(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(*output, "plan.json"))
	if e != nil {
		t.Fatal(e)
	}
	var p plan
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	if p.Snapshot != 1 || len(p.Records) != 1 || len(p.Memories) != 1 || p.Records[0].Text != "past" {
		t.Fatal("boundary changed", p)
	}
	if e = run(); e == nil {
		t.Fatal("frozen plan overwritten")
	}
	*applyPlan = filepath.Join(*output, "plan.json")
	if _, e = db.Exec(`UPDATE history SET payload='changed' WHERE seq=1`); e != nil {
		t.Fatal(e)
	}
	if e = apply(db); e == nil || !strings.Contains(e.Error(), "source differs") {
		t.Fatal("changed source accepted", e)
	}
}
