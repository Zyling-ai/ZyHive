package channel

import "testing"

func TestSniffImageContentType(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg magic", []byte{0xff, 0xd8, 0xff, 0xe0}, "image/jpeg"},
		{"png magic", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, "image/png"},
		{"gif87a", []byte("GIF87a"), "image/gif"},
		{"webp", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}, "image/webp"},
		{"unknown falls to jpeg", []byte{0x00, 0x11, 0x22, 0x33}, "image/jpeg"},
		{"empty falls to jpeg", nil, "image/jpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sniffImageContentType(tc.data)
			if got != tc.want {
				t.Fatalf("sniffImageContentType(%v) = %q, want %q", tc.data, got, tc.want)
			}
		})
	}
}

func TestExtFromContentType(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":               ".jpg",
		"image/png":                ".png",
		"image/gif":                ".gif",
		"image/webp":               ".webp",
		"application/octet-stream": "",
		"":                         "",
	}
	for ct, want := range cases {
		if got := extFromContentType(ct); got != want {
			t.Fatalf("extFromContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}
