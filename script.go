//go:build windows

package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
)

var tagRe = regexp.MustCompile("<[^>]+>")
var multiSpaceRe = regexp.MustCompile("[ \\t\\r]+")
var multiNLRe = regexp.MustCompile("\\n{3,}")

func readScript(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt":
		return readTextFile(path)
	case ".docx":
		return readDocx(path)
	case ".doc":
		return readLegacyDoc(path)
	default:
		return "", fmt.Errorf("不支持的文稿格式")
	}
}

func readTextFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil { return "", err }
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		u := make([]uint16, 0, (len(b)-2)/2)
		for i := 2; i+1 < len(b); i += 2 { u = append(u, binary.LittleEndian.Uint16(b[i:i+2])) }
		return cleanScript(string(utf16.Decode(u))), nil
	}
	if len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff {
		u := make([]uint16, 0, (len(b)-2)/2)
		for i := 2; i+1 < len(b); i += 2 { u = append(u, binary.BigEndian.Uint16(b[i:i+2])) }
		return cleanScript(string(utf16.Decode(u))), nil
	}
	if len(b) >= 3 && bytes.Equal(b[:3], []byte{0xef,0xbb,0xbf}) { b = b[3:] }
	if utf8.Valid(b) { return cleanScript(string(b)), nil }
	ps := fmt.Sprintf("[Console]::OutputEncoding=[Text.Encoding]::UTF8; [IO.File]::ReadAllText('%s',[Text.Encoding]::Default)", psQuote(path))
	return runPowerShellText(ps)
}

func readDocx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil { return "", err }
	defer z.Close()
	var data []byte
	for _, f := range z.File {
		if f.Name != "word/document.xml" { continue }
		rc, err := f.Open()
		if err != nil { return "", err }
		data, err = io.ReadAll(rc)
		rc.Close()
		if err != nil { return "", err }
		break
	}
	if len(data) == 0 { return "", fmt.Errorf("DOCX 中没有找到正文") }
	s := string(data)
	s = strings.ReplaceAll(s, "</w:p>", "\n")
	s = strings.ReplaceAll(s, "<w:tab/>", "\t")
	s = strings.ReplaceAll(s, "<w:br/>", "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return cleanScript(s), nil
}

func readLegacyDoc(path string) (string, error) {
	ps := fmt.Sprintf(
		"$w=$null;$d=$null; try { "+
		"$w=New-Object -ComObject Word.Application; "+
		"$w.Visible=$false; "+
		"$d=$w.Documents.Open('%s',$false,$true); "+
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; "+
		"[Console]::Write($d.Content.Text) "+
		"} finally { if($d){$d.Close([ref]$false)}; if($w){$w.Quit()} }",
		psQuote(path))
	t, err := runPowerShellText(ps)
	if err != nil { return "", fmt.Errorf("读取 DOC 失败；该格式需要本机 Microsoft Word 支持：%v", err) }
	return cleanScript(t), nil
}

func runPowerShellText(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true}
	out, err := cmd.Output()
	if err != nil { return "", err }
	return string(bytes.TrimPrefix(out, []byte{0xef,0xbb,0xbf})), nil
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func cleanScript(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = multiNLRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func splitScript(s string, maxRunes int) []string {
	s = cleanScript(s)
	if maxRunes < 8 { maxRunes = 22 }
	var result []string
	var cur []rune
	flush := func() {
		t := strings.TrimSpace(string(cur))
		if t != "" { result = append(result, t) }
		cur = nil
	}
	soft := "，、；：,;:"
	hard := "。！？!?\n"
	for _, r := range []rune(s) {
		cur = append(cur, r)
		n := len(cur)
		if strings.ContainsRune(hard, r) {
			flush()
			continue
		}
		if n >= maxRunes && strings.ContainsRune(soft, r) {
			flush()
			continue
		}
		if n >= maxRunes+6 {
			cut := -1
			for i := len(cur)-2; i >= maxRunes/2; i-- {
				if strings.ContainsRune(soft, cur[i]) { cut = i+1; break }
			}
			if cut > 0 {
				left := strings.TrimSpace(string(cur[:cut]))
				if left != "" { result = append(result, left) }
				cur = append([]rune(nil), cur[cut:]...)
			} else {
				flush()
			}
		}
	}
	flush()
	return rebalanceShort(result, maxRunes)
}

func rebalanceShort(in []string, max int) []string {
	if len(in) < 2 { return in }
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		if len([]rune(out[i])) <= 4 {
			prev := []rune(out[i-1])
			cur := []rune(out[i])
			if len(prev)+len(cur) <= max+6 {
				out[i-1] = strings.TrimSpace(out[i-1] + out[i])
				out = append(out[:i], out[i+1:]...)
				i--
			}
		}
	}
	return out
}
