//go:build windows

package main

import (
	"strings"
	"unicode"
)

type Subtitle struct {
	StartMS int64
	EndMS   int64
	Text    string
}

func normalizeForAlign(s string) string {
	var b strings.Builder
	for _, r := range []rune(strings.ToLower(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || (r >= 0x4e00 && r <= 0x9fff) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func alignScript(chunks []string, segs []RecSegment) []Subtitle {
	if len(chunks) == 0 || len(segs) == 0 { return nil }
	recLens := make([]int, len(segs))
	recCum := make([]int, len(segs)+1)
	for i,s := range segs {
		recLens[i] = len([]rune(normalizeForAlign(s.Text)))
		if recLens[i] < 1 { recLens[i] = 1 }
		recCum[i+1] = recCum[i] + recLens[i]
	}
	scrCum := make([]int, len(chunks)+1)
	for i,s := range chunks {
		n := len([]rune(normalizeForAlign(s)))
		if n < 1 { n = 1 }
		scrCum[i+1] = scrCum[i] + n
	}
	totalScr := scrCum[len(scrCum)-1]
	totalRec := recCum[len(recCum)-1]
	first := segs[0].StartMS
	last := segs[len(segs)-1].EndMS
	if last <= first { last = first + int64(totalScr)*100 }

	mapPos := func(pos int) int64 {
		if totalScr <= 0 { return first }
		target := float64(pos) / float64(totalScr) * float64(totalRec)
		for i := 0; i < len(segs); i++ {
			if target <= float64(recCum[i+1]) {
				den := float64(recLens[i])
				f := 0.0
				if den > 0 { f = (target - float64(recCum[i])) / den }
				if f < 0 { f = 0 }; if f > 1 { f = 1 }
				return segs[i].StartMS + int64(float64(segs[i].EndMS-segs[i].StartMS)*f)
			}
		}
		return last
	}

	out := make([]Subtitle, 0, len(chunks))
	for i,t := range chunks {
		st := mapPos(scrCum[i])
		en := mapPos(scrCum[i+1])
		if i == 0 && st < first { st = first }
		if en <= st { en = st + 500 }
		out = append(out, Subtitle{StartMS:st, EndMS:en, Text:t})
	}
	for i := range out {
		if i > 0 && out[i].StartMS < out[i-1].EndMS {
			out[i].StartMS = out[i-1].EndMS
		}
		if out[i].EndMS <= out[i].StartMS+250 {
			out[i].EndMS = out[i].StartMS + 500
		}
		if i+1 < len(out) && out[i].EndMS > out[i+1].StartMS {
			out[i].EndMS = out[i+1].StartMS
		}
	}
	if len(out) > 0 && out[len(out)-1].EndMS > last+1500 {
		out[len(out)-1].EndMS = last
	}
	return out
}
