package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHTMLText(t *testing.T) {
	zwsp := string(rune(0x200b))
	in := "<p>" + zwsp + "</p><!-- draft --><h2>Notes</h2><ul><li><p>[Abyss]</p></li><li><p>Fixes &amp; more</p></li></ul><p>Bye<br>now</p>"
	want := "Notes\n\n- [Abyss]\n\n- Fixes & more\n\nBye\nnow"
	if got := htmlText(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPatchNotesText(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "patchnotes.html"))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "patchnotes.txt", []byte(htmlText(string(body))+"\n"))
}

func TestHTMLTextLinks(t *testing.T) {
	for in, want := range map[string]string{
		`<p>See <a href="https://aion2.plaync.com/notice" target="_blank"><b>the notice</b></a></p>`: "See the notice (https://aion2.plaync.com/notice)",
		`<a href="https://aion2.plaync.com/x">https://aion2.plaync.com/x</a>`:                        "https://aion2.plaync.com/x",
		`<a href="#alpha-어비스-2">어비스</a>`:                                                             "어비스",
	} {
		if got := htmlText(in); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}
