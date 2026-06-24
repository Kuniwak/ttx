package jsonl

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestWriteLineAppendsNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteLine(&buf, []byte(`{"k":"v"}`)); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != `{"k":"v"}`+"\n" {
		t.Fatalf("WriteLine = %q", got)
	}
}

func TestReadLineReturnsLineWithoutNewline(t *testing.T) {
	r := strings.NewReader("first line\nsecond line\n")
	line, err := ReadLine(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(line) != "first line" {
		t.Fatalf("ReadLine = %q, want %q", line, "first line")
	}
}

func TestReadLineEOFOnEmpty(t *testing.T) {
	_, err := ReadLine(strings.NewReader(""))
	if err != io.EOF {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := []byte(`{"type":"postmortem","text":"1行目\n2行目"}`)
	if err := WriteLine(&buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLine(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round trip = %q, want %q", got, want)
	}
}
