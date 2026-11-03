package controller

import (
	"encoding/binary"
	"testing"
)

func pngChunk(typ string, payload []byte) []byte {
	buf := make([]byte, 0, 12+len(payload))
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(payload)))
	buf = append(buf, l[:]...)
	buf = append(buf, typ...)
	buf = append(buf, payload...)
	buf = append(buf, 0, 0, 0, 0) // CRC 不参与检测，置零
	return buf
}

func pngFile(chunkTypes ...string) []byte {
	data := append([]byte{}, "\x89PNG\r\n\x1a\n"...)
	for _, t := range chunkTypes {
		payload := make([]byte, 13)
		if t == "IHDR" || t == "acTL" || t == "fcTL" {
			if t == "acTL" {
				payload = make([]byte, 8)
			} else if t == "fcTL" {
				payload = make([]byte, 26)
			}
		}
		data = append(data, pngChunk(t, payload)...)
	}
	return data
}

func TestIsLikelyAnimatedRaster(t *testing.T) {
	webpAnim := append([]byte{}, "RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00"...)
	webpAnim = append(webpAnim, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	webpStatic := append([]byte{}, "RIFF\x00\x00\x00\x00WEBPVP8 "...)
	webpStatic = append(webpStatic, 0, 0, 0, 0, 0)

	cases := []struct {
		name string
		head []byte
		want bool
	}{
		{"gif89a", append([]byte("GIF89a"), make([]byte, 64)...), true},
		{"gif87a", append([]byte("GIF87a"), make([]byte, 64)...), true},
		{"animated webp", webpAnim, true},
		{"static webp", webpStatic, false},
		{"apng", pngFile("IHDR", "acTL", "fcTL", "IDAT"), true},
		{"apng fctl after actl", pngFile("IHDR", "acTL", "fcTL", "IDAT"), true},
		{"static png", pngFile("IHDR", "IDAT", "IEND"), false},
		{"acTL after IDAT not counted", pngFile("IHDR", "IDAT", "acTL"), false},
		{"jpeg", append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...), false},
		{"too short", []byte("GIF8"), false},
		{"empty", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLikelyAnimatedRaster(tc.head); got != tc.want {
				t.Fatalf("isLikelyAnimatedRaster(%q...) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
