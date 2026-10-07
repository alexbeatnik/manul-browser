// Package data implements data-driven testing support for Manul Browser.
//
// It loads JSON arrays or CSV files referenced by @data: directives
// and feeds each row into the runtime as scoped variables.
package data

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Row is a single data-driven test row (map of string → string).
type Row = map[string]string

// LoadFile reads a JSON array-of-objects or CSV file and returns rows.
// Resolution order: relative to huntDir, then CWD.
func LoadFile(dataPath string, huntDir string) ([]Row, error) {
	candidates := []string{
		filepath.Join(huntDir, dataPath),
		filepath.Join(huntDir, "..", dataPath),
		filepath.Join(huntDir, "data", dataPath),
		filepath.Join(huntDir, "..", "data", dataPath),
		dataPath,
	}

	var resolved string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			resolved = c
			break
		}
	}
	if resolved == "" {
		return nil, fmt.Errorf("data file not found: %s (searched in hunt dir, ../data/, and CWD)", dataPath)
	}

	if filepath.Ext(resolved) == ".json" {
		return loadJSON(resolved)
	}
	if filepath.Ext(resolved) == ".csv" {
		return loadCSV(resolved)
	}
	return nil, fmt.Errorf("unsupported data file type: %s (use .json or .csv)", dataPath)
}

// RowsFor returns the rows a hunt runs once for each of.
//
// It is never empty. A hunt with no @data: file runs once with nothing extra,
// and so does one whose file holds no rows — a hunt that ran zero times has
// nothing to fail, and reports a pass. Every path that runs a hunt asks here,
// so `manul run`, the worker pool, a session and the daemon agree on what
// data-driven means.
func RowsFor(dataFile, huntPath string) ([]Row, error) {
	if dataFile == "" {
		return []Row{nil}, nil
	}
	rows, err := LoadFile(dataFile, filepath.Dir(huntPath))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Row{nil}, nil
	}
	return rows, nil
}

func loadJSON(path string) ([]Row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Numbers are kept as written. Decoded to float64 and printed, an order id
	// of 1234567 reaches the form as "1.234567e+06".
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw []map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	rows := make([]Row, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		row := make(Row, len(item))
		for k, v := range item {
			if v == nil {
				row[k] = ""
			} else {
				row[k] = fmt.Sprint(v)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func loadCSV(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV in %s: %w", path, err)
	}
	if len(records) < 2 {
		return nil, nil // header only or empty
	}

	headers := make([]string, len(records[0]))
	for i, h := range records[0] {
		headers[i] = strings.TrimSpace(h)
	}
	rows := make([]Row, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(Row, len(headers))
		for i, h := range headers {
			if i < len(record) {
				row[h] = strings.TrimSpace(record[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
