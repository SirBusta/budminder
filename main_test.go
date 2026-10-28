package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestBackupDB_SpecialFolderName: the pre-update backup works from a
// folder whose name has # or % (they mean something in a file: URI).
func TestBackupDB_SpecialFolderName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	plain := filepath.Join(root, "isley.db")
	db, err := sql.Open("sqlite", plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE t (v TEXT); INSERT INTO t VALUES ('kept')"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	dir := filepath.Join(root, "Grow #2 100%")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "isley.db")
	if err := os.Rename(plain, src); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(root, "backup.db")
	if err := backupDB(src, dst); err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	out, err := sql.Open("sqlite", dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var v string
	if err := out.QueryRow("SELECT v FROM t").Scan(&v); err != nil || v != "kept" {
		t.Fatalf("backup content = %q, %v", v, err)
	}
}
