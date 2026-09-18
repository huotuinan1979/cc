//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	idAudio = 101
	idScript = 102
	idGenerate = 103
	idFormat = 104
	idOpen = 105
)

var (
	mainHwnd uintptr
	hAudio, hScript, hGenerate, hFormat, hProgress, hStatus, hOpen uintptr
	hTitle, hHint uintptr
	hInst uintptr

	audioPath string
	scriptPath string
	lastOutput string
	busy bool
	uiMu sync.Mutex

	formatNames = []string{
		"Final Cut Pro XML (.fcpxml)",
		"Final Cut Pro 字幕 (.itt)",
		"通用字幕 (.srt)",
		"旧版 Final Cut XML (.xml)",
		"透明字幕图片 (PNG 序列)",
		"字幕时间表 (.csv)",
	}
)

func main() {
	pInitCommonControls.Call()
	h, _, _ := pGetModuleHandleW.Call(0)
	hInst = h

	className := utf16("NineZeroSixSubtitleV14")
	cursor, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEX{
		CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc: syscall.NewCallback(wndProc),
		HInstance: hInst,
		HCursor: cursor,
		HbrBackground: 6,
		LpszClassName: className,
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	mainHwnd = createWindow(0, "NineZeroSixSubtitleV14", "玖零六影视字幕生成工具 V1.4", WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100, 100, 820, 470, 0, 0, hInst)
	if mainHwnd == 0 {
		panic("无法创建窗口")
	}
	pDragAcceptFiles.Call(mainHwnd, 1)
	pShowWindow.Call(mainHwnd, SW_SHOW)
	pUpdateWindow.Call(mainHwnd)

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 { break }
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		createControls(hwnd)
		return 0
	case WM_SIZE:
		layoutControls(hwnd)
		return 0
	case WM_COMMAND:
		id := int(loword(wParam))
		code := int(hiword(wParam))
		switch {
		case id == idAudio && code == BN_CLICKED:
			p := chooseFile(hwnd, "音频文件\x00*.wav;*.mp3;*.m4a;*.aac;*.flac;*.ogg;*.wma\x00所有文件\x00*.*\x00", "选择配音文件")
			if p != "" { setInputFile(p) }
		case id == idScript && code == BN_CLICKED:
			p := chooseFile(hwnd, "文稿文件\x00*.txt;*.doc;*.docx\x00所有文件\x00*.*\x00", "选择解说词文稿")
			if p != "" { setInputFile(p) }
		case id == idGenerate && code == BN_CLICKED:
			startGenerate()
		case id == idFormat && code == CBN_SELCHANGE:
			// 只记录选择；生成时读取。
		case id == idOpen && code == BN_CLICKED:
			openResult()
		}
		return 0
	case WM_DROPFILES:
		handleDrop(wParam)
		return 0
	case WM_APP_UPDATE:
		refreshTaskUI()
		return 0
	case WM_APP_DONE:
		refreshTaskUI()
		pEnableWindow.Call(hGenerate, 1)
		return 0
	case WM_CLOSE:
		cancelRunningProcess()
		pDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		cancelRunningProcess()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func createControls(hwnd uintptr) {
	hTitle = createWindow(0, "STATIC", "玖零六影视字幕生成工具", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 18, 500, 30, hwnd, 0, hInst)
	hHint = createWindow(0, "STATIC", "拖入配音文件和解说词文稿，然后选择输出格式并生成", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 52, 680, 24, hwnd, 0, hInst)

	createWindow(0, "STATIC", "配音文件", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 100, 100, 24, hwnd, 0, hInst)
	hAudio = createWindow(WS_EX_CLIENTEDGE, "EDIT", "未导入", WS_CHILD|WS_VISIBLE|WS_BORDER|ES_AUTOHSCROLL, 124, 96, 550, 28, hwnd, 0, hInst)
	createWindow(0, "BUTTON", "选择", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, 688, 96, 82, 28, hwnd, idAudio, hInst)

	createWindow(0, "STATIC", "解说词", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 142, 100, 24, hwnd, 0, hInst)
	hScript = createWindow(WS_EX_CLIENTEDGE, "EDIT", "未导入", WS_CHILD|WS_VISIBLE|WS_BORDER|ES_AUTOHSCROLL, 124, 138, 550, 28, hwnd, 0, hInst)
	createWindow(0, "BUTTON", "选择", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, 688, 138, 82, 28, hwnd, idScript, hInst)

	createWindow(0, "STATIC", "输出格式", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 190, 100, 24, hwnd, 0, hInst)
	hFormat = createWindow(0, "COMBOBOX", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|CBS_DROPDOWNLIST, 124, 184, 360, 240, hwnd, idFormat, hInst)
	for _, name := range formatNames {
		send(hFormat, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16(name))))
	}
	send(hFormat, CB_SETCURSEL, 0, 0)

	hGenerate = createWindow(0, "BUTTON", "生成字幕", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, 510, 182, 160, 34, hwnd, idGenerate, hInst)

	hProgress = createWindow(0, "msctls_progress32", "", WS_CHILD|WS_VISIBLE, 24, 248, 746, 22, hwnd, 0, hInst)
	send(hProgress, PBM_SETRANGE32, 0, 100)
	send(hProgress, PBM_SETPOS, 0, 0)
	hStatus = createWindow(0, "STATIC", "待机", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 280, 650, 28, hwnd, 0, hInst)
	hOpen = createWindow(0, "BUTTON", "打开结果", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, 650, 276, 120, 30, hwnd, idOpen, hInst)
	pEnableWindow.Call(hOpen, 0)

	createWindow(0, "STATIC", "说明：音频只用于卡时间与校对，最终字幕文字始终以导入的解说词为准。", WS_CHILD|WS_VISIBLE|SS_LEFT, 24, 338, 746, 48, hwnd, 0, hInst)
}

func layoutControls(hwnd uintptr) {
	var rc RECT
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left
	if w < 650 { w = 650 }
	editW := w - 270
	if editW < 300 { editW = 300 }
	pMoveWindow.Call(hAudio, 124, 96, uintptr(editW), 28, 1)
	pMoveWindow.Call(hScript, 124, 138, uintptr(editW), 28, 1)
	pMoveWindow.Call(hProgress, 24, 248, uintptr(w-48), 22, 1)
}

func handleDrop(hdrop uintptr) {
	count, _, _ := pDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	for i := uintptr(0); i < count; i++ {
		n, _, _ := pDragQueryFileW.Call(hdrop, i, 0, 0)
		buf := make([]uint16, n+1)
		pDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), n+1)
		setInputFile(syscall.UTF16ToString(buf))
	}
	pDragFinish.Call(hdrop)
}

func setInputFile(p string) {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".wma":
		audioPath = p
		setText(hAudio, p)
		updateStatus(5, "已导入配音："+filepath.Base(p), false, "")
	case ".txt", ".doc", ".docx":
		scriptPath = p
		setText(hScript, p)
		updateStatus(5, "已导入解说词："+filepath.Base(p), false, "")
	}
}

func selectedFormat() int {
	i := int(send(hFormat, CB_GETCURSEL, 0, 0))
	if i < 0 || i >= len(formatNames) { return 0 }
	return i
}

func startGenerate() {
	uiMu.Lock()
	if busy {
		uiMu.Unlock()
		return
	}
	a, s := audioPath, scriptPath
	uiMu.Unlock()
	if a == "" || s == "" {
		updateStatus(0, "请先导入配音文件和解说词文稿。", false, "")
		return
	}
	idx := selectedFormat()
	pEnableWindow.Call(hGenerate, 0)
	pEnableWindow.Call(hOpen, 0)
	updateStatus(1, "正在准备…", false, "")
	go func() {
		uiMu.Lock()
		busy = true
		uiMu.Unlock()
		out, err := runJob(a, s, idx)
		uiMu.Lock()
		busy = false
		if err == nil { lastOutput = out }
		uiMu.Unlock()
		if err != nil {
			updateStatus(0, "处理失败："+err.Error(), true, "")
		} else {
			updateStatus(100, "生成完成："+filepath.Base(out), true, out)
		}
		pPostMessageW.Call(mainHwnd, WM_APP_DONE, 0, 0)
	}()
}

var taskProgress int
var taskText string
var taskDone bool
var taskOutput string

func updateStatus(progress int, text string, done bool, output string) {
	uiMu.Lock()
	taskProgress, taskText, taskDone, taskOutput = progress, text, done, output
	uiMu.Unlock()
	if mainHwnd != 0 { pPostMessageW.Call(mainHwnd, WM_APP_UPDATE, 0, 0) }
}

func refreshTaskUI() {
	uiMu.Lock()
	p, t, done, out := taskProgress, taskText, taskDone, taskOutput
	uiMu.Unlock()
	send(hProgress, PBM_SETPOS, uintptr(p), 0)
	setText(hStatus, fmt.Sprintf("%s  %d%%", t, p))
	if done && out != "" {
		pEnableWindow.Call(hOpen, 1)
	}
}

func openResult() {
	uiMu.Lock()
	p := lastOutput
	uiMu.Unlock()
	if p == "" { return }
	target := p
	if st, err := os.Stat(p); err == nil && !st.IsDir() { target = filepath.Dir(p) }
	execHidden("explorer.exe", target)
}
