package xmind

import (
	"html"
	"strings"
	"testing"
)

func TestNotesFromPlainText(t *testing.T) {
	if n := NotesFromPlainText(""); n != nil {
		t.Fatalf("NotesFromPlainText(\"\") = %+v, want nil", n)
	}
	n := NotesFromPlainText("a & b\nc")
	if n == nil || n.Plain == nil || n.RealHTML == nil {
		t.Fatalf("NotesFromPlainText: want both bodies, got %+v", n)
	}
	if n.Plain.Content != "a & b\nc" {
		t.Errorf("Plain.Content = %q, want the input", n.Plain.Content)
	}
	if want := "<div>a &amp; b</div><div>c</div>"; n.RealHTML.Content != want {
		t.Errorf("RealHTML.Content = %q, want %q", n.RealHTML.Content, want)
	}
	if n.extra != nil || n.Plain.extra != nil || n.RealHTML.extra != nil {
		t.Errorf("NotesFromPlainText: want no extra keys, got %+v", n)
	}
}

func TestPlainToRealHTML(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"a", "<div>a</div>"},
		{`&<>"'`, "<div>&amp;&lt;&gt;&#34;&#39;</div>"},
		{"a\n\nb", "<div>a</div><div></div><div>b</div>"},
		{"a\n", "<div>a</div><div></div>"},
	} {
		if got := plainToRealHTML(tc.in); got != tc.want {
			t.Errorf("plainToRealHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// FuzzPlainToRealHTML checks that plainToRealHTML loses nothing: undoing the <div> wrapping and
// the HTML escaping gives back the plain text.
func FuzzPlainToRealHTML(f *testing.F) {
	for _, s := range []string{"", "a", "a\nb", "\n", "a\n", `&<>"'`, "</div><div>", "&amp;lt;"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := plainToRealHTML(s)
		if (out == "") != (s == "") {
			t.Fatalf("plainToRealHTML(%q) = %q", s, out)
		}
		if s == "" {
			return
		}
		inner, okPrefix := strings.CutPrefix(out, "<div>")
		inner, okSuffix := strings.CutSuffix(inner, "</div>")
		if !okPrefix || !okSuffix {
			t.Fatalf("plainToRealHTML(%q) = %q, want it wrapped in <div>...</div>", s, out)
		}
		if back := html.UnescapeString(strings.ReplaceAll(inner, "</div><div>", "\n")); back != s {
			t.Fatalf("plainToRealHTML(%q) = %q, which decodes to %q", s, out, back)
		}
	})
}
