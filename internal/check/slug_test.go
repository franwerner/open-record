package check

import "testing"

func TestSlugConvertsTextToItsAnchor(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
	}{
		"the worked example from the spec": {
			text: "Archivo muy grande (más de 50 MB)",
			want: "archivo-muy-grande-mas-de-50-mb",
		},
		"accents and diacritics are stripped, uppercase ones too": {
			text: "Mañana, el Ñandú se IRÁ",
			want: "manana-el-nandu-se-ira",
		},
		"an already-decomposed accent is stripped the same way": {
			text: "ma\u0301s",
			want: "mas",
		},
		"a letter with no decomposition is kept as written": {
			text: "Straße",
			want: "straße",
		},
		"uppercase is lowercased": {
			text: "PENDING",
			want: "pending",
		},
		"punctuation becomes a hyphen": {
			text: "pending, paid!",
			want: "pending-paid",
		},
		"repeated separators collapse to one hyphen": {
			text: "a   -  b",
			want: "a-b",
		},
		"leading and trailing punctuation is trimmed, not just collapsed": {
			text: "-- already an anchor --",
			want: "already-an-anchor",
		},
		"digits survive": {
			text: "Over 50 MB",
			want: "over-50-mb",
		},
		"a name with no letters or digits slugs to the empty string": {
			text: "!!!",
			want: "",
		},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Slug(item.text); got != item.want {
				t.Errorf("Slug(%q) = %q, want %q", item.text, got, item.want)
			}
		})
	}
}

func TestSlugIsDeterministic(t *testing.T) {
	// A caller may Slug the same name twice — once to write a reference, once
	// to check it still resolves — and the two had better agree.
	text := "Pending → Paid (the payment is confirmed)"
	if first, second := Slug(text), Slug(text); first != second {
		t.Errorf("Slug was not deterministic: %q then %q", first, second)
	}
}
