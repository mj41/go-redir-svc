package main

// Per-group log files, one per UTC day: <log_file without .jsonl>-<YYYY-MM-DD>-<host>.jsonl. A day's
// file is never written again once the day is over, so whatever ships logs elsewhere (in the cluster:
// a sidecar moving them to Object Storage) can take any file not of today. The host part keeps two
// pods (or a pod and its replacement) from writing the same name.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type dayLog struct {
	dir, base, host string
	day             string // the open file's UTC day, "" when none is open
	f               *os.File
}

func newDayLog(dir, logFile, host string) *dayLog {
	return &dayLog{dir: dir, base: strings.TrimSuffix(logFile, ".jsonl"), host: host}
}

// name is the file for day (YYYY-MM-DD).
func (d *dayLog) name(day string) string {
	return filepath.Join(d.dir, fmt.Sprintf("%s-%s-%s.jsonl", d.base, day, d.host))
}

// write appends line to now's day file, opening it (and closing the day before) when the day changed.
func (d *dayLog) write(now time.Time, line []byte) error {
	day := now.UTC().Format("2006-01-02")
	if d.f == nil || d.day != day {
		if d.f != nil {
			d.f.Close()
			d.f, d.day = nil, ""
		}
		f, err := os.OpenFile(d.name(day), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		d.f, d.day = f, day
	}
	_, err := d.f.Write(line)
	return err
}

func (d *dayLog) Close() error {
	if d.f == nil {
		return nil
	}
	err := d.f.Close()
	d.f, d.day = nil, ""
	return err
}

// hostName is the pod's name (os.Hostname), made safe for a file name.
func hostName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "host"
	}
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, h)
}
