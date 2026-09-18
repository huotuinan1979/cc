//go:build windows

package main

import (
	"fmt"
)

func runJob(audio, script string, format int) (string, error) {
	updateStatus(3, "正在检查本地组件…", false, "")
	root := runtimeRoot()

	updateStatus(8, "正在读取解说词…", false, "")
	text, err := readScript(script)
	if err != nil { return "", err }
	chunks := splitScript(text, 22)
	if len(chunks) == 0 { return "", fmt.Errorf("解说词为空") }

	updateStatus(18, "正在分析配音…", false, "")
	segs, err := recognizeAudio(audio, root)
	if err != nil { return "", err }

	updateStatus(74, "正在匹配解说词与时间轴…", false, "")
	subs := alignScript(chunks, segs)
	if len(subs) == 0 { return "", fmt.Errorf("无法建立字幕时间轴") }

	updateStatus(88, "正在生成所选格式…", false, "")
	out, err := exportResult(subs, audio, format)
	if err != nil { return "", err }

	updateStatus(98, "正在完成…", false, "")
	return out, nil
}
