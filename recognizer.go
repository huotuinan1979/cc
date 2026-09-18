//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type RecSegment struct {
	StartMS int64
	EndMS   int64
	Text    string
}

type whisperJSON struct {
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

var progressRe = regexp.MustCompile(`(?i)progress[^0-9]*(\d{1,3})%`)

func recognizeAudio(audio, root string) ([]RecSegment, error) {
	whisper, err := findRecursive(filepath.Join(root, "whisper"), "whisper-cli.exe")
	if err != nil { return nil, err }
	model, err := componentPath(root, "ggml-base-q5_1.bin")
	if err != nil { return nil, err }

	input := audio
	switch strings.ToLower(filepath.Ext(audio)) {
	case ".wav", ".mp3", ".flac", ".ogg":
		// whisper-cli 可直接读取。
	default:
		ffmpeg, e := componentPath(root, "ffmpeg.exe")
		if e != nil { return nil, e }
		wav := filepath.Join(os.TempDir(), "906_subtitle_audio.wav")
		updateStatus(12, "正在转换音频…", false, "")
		if err := runHidden(ffmpeg, "-y", "-i", audio, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav); err != nil {
			return nil, fmt.Errorf("音频转换失败：%v", err)
		}
		defer os.Remove(wav)
		input = wav
	}

	tmp, err := os.MkdirTemp("", "906subtitle-")
	if err != nil { return nil, err }
	defer os.RemoveAll(tmp)
	prefix := filepath.Join(tmp, "recognition")

	threads := runtime.NumCPU() - 1
	if threads < 2 { threads = 2 }
	args := []string{"-m", model, "-f", input, "-l", "zh", "-oj", "-of", prefix, "-pp", "-t", strconv.Itoa(threads)}
	cmd := exec.Command(whisper, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true}
	stderr, err := cmd.StderrPipe()
	if err != nil { return nil, err }
	stdout, err := cmd.StdoutPipe()
	if err != nil { return nil, err }

	processMu.Lock()
	currentCmd = cmd
	processMu.Unlock()
	if err := cmd.Start(); err != nil {
		processMu.Lock(); currentCmd = nil; processMu.Unlock()
		return nil, fmt.Errorf("识别引擎启动失败：%v", err)
	}
	go io.Copy(io.Discard, stdout)
	sc := bufio.NewScanner(stderr)
	for sc.Scan() {
		line := sc.Text()
		m := progressRe.FindStringSubmatch(line)
		if len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			if n < 0 { n = 0 }; if n > 100 { n = 100 }
			updateStatus(20+n*50/100, fmt.Sprintf("正在识别配音… %d%%", n), false, "")
		}
	}
	err = cmd.Wait()
	processMu.Lock()
	if currentCmd == cmd { currentCmd = nil }
	processMu.Unlock()
	if err != nil { return nil, fmt.Errorf("配音识别失败：%v", err) }

	b, err := os.ReadFile(prefix + ".json")
	if err != nil { return nil, fmt.Errorf("识别结果读取失败：%v", err) }
	var js whisperJSON
	if err := json.Unmarshal(b, &js); err != nil { return nil, fmt.Errorf("识别结果解析失败：%v", err) }
	var segs []RecSegment
	for _, s := range js.Transcription {
		t := strings.TrimSpace(s.Text)
		if t == "" || s.Offsets.To <= s.Offsets.From { continue }
		segs = append(segs, RecSegment{StartMS:s.Offsets.From, EndMS:s.Offsets.To, Text:t})
	}
	if len(segs) == 0 { return nil, fmt.Errorf("没有识别到有效语音") }
	return segs, nil
}

func findRecursive(root, name string) (string, error) {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil { return nil }
		if !info.IsDir() && strings.EqualFold(info.Name(), name) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" { return "", fmt.Errorf("本地组件缺失：%s", name) }
	return found, nil
}
