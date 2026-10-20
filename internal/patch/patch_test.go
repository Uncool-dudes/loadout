package patch

import (
	"errors"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

func TestInvocation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		inv  skill.Invocation
		want string
	}{
		{
			name: "user only",
			src:  "---\nname: x\ndescription: d\n---\n\nbody\n",
			inv:  skill.User,
			want: "---\nname: x\ndescription: d\ndisable-model-invocation: true\nuser-invocable: true\n---\n\nbody\n",
		},
		{
			name: "model only",
			src:  "---\nname: x\n---\nbody\n",
			inv:  skill.Model,
			want: "---\nname: x\ndisable-model-invocation: false\nuser-invocable: false\n---\nbody\n",
		},
		{
			name: "replaces the author's own keys",
			src:  "---\nname: x\nuser-invocable: false\ndescription: d\ndisable-model-invocation: true\n---\nbody\n",
			inv:  skill.User,
			want: "---\nname: x\ndescription: d\ndisable-model-invocation: true\nuser-invocable: true\n---\nbody\n",
		},
		{
			name: "both leaves the file as authored",
			src:  "---\nname: x\nuser-invocable: false\n---\nbody\n",
			inv:  skill.Both,
			want: "---\nname: x\nuser-invocable: false\n---\nbody\n",
		},
		{
			name: "CRLF line endings are kept",
			src:  "---\r\nname: x\r\n---\r\nbody\r\n",
			inv:  skill.User,
			want: "---\r\nname: x\r\ndisable-model-invocation: true\r\nuser-invocable: true\r\n---\r\nbody\r\n",
		},
		{
			name: "a dashed rule in the body is not the header end",
			src:  "---\nname: x\n---\nbody\n---\nmore\n",
			inv:  skill.User,
			want: "---\nname: x\ndisable-model-invocation: true\nuser-invocable: true\n---\nbody\n---\nmore\n",
		},
		{
			name: "similar key names are untouched",
			src:  "---\nname: x\nuser-invocable-hint: keep\n---\n",
			inv:  skill.Model,
			want: "---\nname: x\nuser-invocable-hint: keep\ndisable-model-invocation: false\nuser-invocable: false\n---\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Invocation([]byte(tt.src), tt.inv)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
			again, err := Invocation(got, tt.inv)
			if err != nil || string(again) != tt.want {
				t.Errorf("not idempotent:\n%q", again)
			}
		})
	}
}

func TestInvocationWithoutFrontmatter(t *testing.T) {
	for _, src := range []string{"no header\n", "---\nname: x\nnever closed\n", ""} {
		if _, err := Invocation([]byte(src), skill.User); !errors.Is(err, ErrNoFrontmatter) {
			t.Errorf("Invocation(%q) error = %v, want ErrNoFrontmatter", src, err)
		}
	}
}
