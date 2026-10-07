package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_JSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "test.json"), []byte(`[
		{"username": "alice", "password": "secret1"},
		{"username": "bob", "password": "secret2"}
	]`), 0644)

	rows, err := LoadFile("test.json", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["username"] != "alice" {
		t.Errorf("row[0].username = %q", rows[0]["username"])
	}
	if rows[1]["password"] != "secret2" {
		t.Errorf("row[1].password = %q", rows[1]["password"])
	}
}

func TestLoadFile_CSV(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "users.csv"), []byte("username,password\nalice,secret1\nbob,secret2\n"), 0644)

	rows, err := LoadFile("users.csv", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["username"] != "alice" {
		t.Errorf("row[0].username = %q", rows[0]["username"])
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("missing.json", t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFile_UnsupportedType(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "data.xml"), []byte("<root/>"), 0644)
	_, err := LoadFile("data.xml", dir)
	if err == nil {
		t.Fatal("expected error for unsupported file type")
	}
}

func TestLoadFile_EmptyJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "empty.json"), []byte("[]"), 0644)
	rows, err := LoadFile("empty.json", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for empty JSON array, got %d", len(rows))
	}
}

func TestLoadFile_EmptyCSV(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "empty.csv"), []byte("username,password\n"), 0644)
	rows, err := LoadFile("empty.csv", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for CSV with only header, got %d", len(rows))
	}
}

func TestLoadFile_CSVWithSpaces(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "spaces.csv"), []byte(" username , password \n alice , secret1 \n"), 0644)
	rows, err := LoadFile("spaces.csv", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["username"] != "alice" {
		t.Errorf("username = %q, want alice", rows[0]["username"])
	}
	if rows[0]["password"] != "secret1" {
		t.Errorf("password = %q, want secret1", rows[0]["password"])
	}
}

func TestLoadFile_JSONObjectInsteadOfArray(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "object.json"), []byte(`{"username": "alice", "password": "secret1"}`), 0644)
	_, err := LoadFile("object.json", dir)
	if err == nil {
		t.Fatal("expected error for JSON object instead of array")
	}
}

func TestLoadFile_CRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "crlf.csv"), []byte("username,password\r\nalice,secret1\r\nbob,secret2\r\n"), 0644)
	rows, err := LoadFile("crlf.csv", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["username"] != "alice" {
		t.Errorf("row[0].username = %q", rows[0]["username"])
	}
}

func TestLoadFile_SingleRowCSV(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "single.csv"), []byte("username,password\nalice,secret1\n"), 0644)
	rows, err := LoadFile("single.csv", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["username"] != "alice" {
		t.Errorf("row[0].username = %q", rows[0]["username"])
	}
}

func TestLoadFile_JSONWithNulls(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "nulls.json"), []byte(`[
		{"username": "alice", "password": null},
		{"username": null, "password": "secret2"}
	]`), 0644)
	rows, err := LoadFile("nulls.json", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["password"] != "" {
		t.Errorf("null value should become empty string, got %q", rows[0]["password"])
	}
	if rows[1]["username"] != "" {
		t.Errorf("null value should become empty string, got %q", rows[1]["username"])
	}
}

func TestLoadFile_JSONWithNumbers(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "numbers.json"), []byte(`[
		{"id": 42, "name": "test"}
	]`), 0644)
	rows, err := LoadFile("numbers.json", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["id"] != "42" {
		t.Errorf("number should become string, got %q", rows[0]["id"])
	}
}

func TestLoadFile_CSVWithEmptyFields(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "emptyfields.csv"), []byte("a,b,c\n1,,3\n"), 0644)
	rows, err := LoadFile("emptyfields.csv", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["a"] != "1" || rows[0]["b"] != "" || rows[0]["c"] != "3" {
		t.Errorf("unexpected values: %v", rows[0])
	}
}

// A number in a data file is typed into a form as text, so it has to come out
// the way it went in. Through float64 and fmt, 1234567 became "1.234567e+06".
func TestLoadFile_JSONNumbersKeepTheirSpelling(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "orders.json"), []byte(`[
		{"id": 1234567, "phone": 380501234567, "price": 19.90, "qty": 3, "big": 9007199254740993, "ok": true, "note": null}
	]`), 0644)

	rows, err := LoadFile("orders.json", dir)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	want := map[string]string{
		"id": "1234567", "phone": "380501234567", "price": "19.90", "qty": "3",
		"big": "9007199254740993", "ok": "true", "note": "",
	}
	for k, v := range want {
		if rows[0][k] != v {
			t.Errorf("%s = %q, want %q", k, rows[0][k], v)
		}
	}
}

// A hunt always runs at least once. An empty data file used to mean zero runs,
// and a hunt that never ran has nothing to fail.
func TestRowsFor(t *testing.T) {
	dir := t.TempDir()
	hunt := filepath.Join(dir, "login.hunt")
	_ = os.WriteFile(filepath.Join(dir, "users.csv"), []byte("user\nann\nbob\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "empty.csv"), []byte("user\n"), 0644)

	rows, err := RowsFor("", hunt)
	if err != nil || len(rows) != 1 || rows[0] != nil {
		t.Errorf("no data file: rows=%v err=%v", rows, err)
	}
	rows, err = RowsFor("empty.csv", hunt)
	if err != nil || len(rows) != 1 || rows[0] != nil {
		t.Errorf("empty data file: rows=%v err=%v", rows, err)
	}
	rows, err = RowsFor("users.csv", hunt)
	if err != nil || len(rows) != 2 || rows[1]["user"] != "bob" {
		t.Errorf("two rows: rows=%v err=%v", rows, err)
	}
	if _, err := RowsFor("missing.csv", hunt); err == nil {
		t.Error("a data file that is not there should be an error")
	}
}
