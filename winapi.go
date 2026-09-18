//go:build windows

package main

import (
	"syscall"
	"unsafe"
	utf16pkg "unicode/utf16"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pShowWindow         = user32.NewProc("ShowWindow")
	pUpdateWindow       = user32.NewProc("UpdateWindow")
	pGetMessageW        = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessageW   = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pSendMessageW       = user32.NewProc("SendMessageW")
	pPostMessageW       = user32.NewProc("PostMessageW")
	pSetWindowTextW     = user32.NewProc("SetWindowTextW")
	pGetWindowTextW     = user32.NewProc("GetWindowTextW")
	pGetWindowTextLenW  = user32.NewProc("GetWindowTextLengthW")
	pEnableWindow       = user32.NewProc("EnableWindow")
	pMoveWindow         = user32.NewProc("MoveWindow")
	pLoadCursorW        = user32.NewProc("LoadCursorW")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pSetFocus           = user32.NewProc("SetFocus")

	pGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")

	pDragAcceptFiles    = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW     = shell32.NewProc("DragQueryFileW")
	pDragFinish         = shell32.NewProc("DragFinish")

	pGetOpenFileNameW   = comdlg32.NewProc("GetOpenFileNameW")
	pInitCommonControls = comctl32.NewProc("InitCommonControls")
	pCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	pCoUninitialize     = ole32.NewProc("CoUninitialize")
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_TABSTOP          = 0x00010000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_EX_CLIENTEDGE    = 0x00000200

	BS_PUSHBUTTON = 0
	SS_LEFT      = 0
	ES_AUTOHSCROLL = 0x0080
	CBS_DROPDOWNLIST = 0x0003

	CW_USEDEFAULT = ^uintptr(0x7fffffff)

	SW_SHOW = 5

	WM_CREATE      = 0x0001
	WM_DESTROY     = 0x0002
	WM_SIZE        = 0x0005
	WM_COMMAND     = 0x0111
	WM_CLOSE       = 0x0010
	WM_DROPFILES   = 0x0233
	WM_APP         = 0x8000
	WM_APP_UPDATE  = WM_APP + 1
	WM_APP_DONE    = WM_APP + 2

	BN_CLICKED = 0
	CBN_SELCHANGE = 1

	PBM_SETRANGE32 = 0x0400 + 6
	PBM_SETPOS     = 0x0400 + 2

	CB_ADDSTRING   = 0x0143
	CB_SETCURSEL   = 0x014E
	CB_GETCURSEL   = 0x0147

	IDC_ARROW = 32512

	OFN_EXPLORER      = 0x00080000
	OFN_FILEMUSTEXIST = 0x00001000
	OFN_PATHMUSTEXIST = 0x00000800
)

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type POINT struct{ X, Y int32 }

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	LPrivate uint32
}

type RECT struct{ Left, Top, Right, Bottom int32 }

type OPENFILENAME struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
}

func utf16(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }

func setText(hwnd uintptr, s string) {
	pSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16(s))))
}

func getText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLenW.Call(hwnd)
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func createWindow(exStyle uint32, class, title string, style uint32, x, y, w, h int32, parent, menu, inst uintptr) uintptr {
	hwnd, _, _ := pCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(utf16(class))),
		uintptr(unsafe.Pointer(utf16(title))),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, menu, inst, 0,
	)
	return hwnd
}

func send(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(hwnd, uintptr(msg), w, l)
	return r
}

func loword(v uintptr) uint16 { return uint16(v & 0xffff) }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xffff) }

func utf16MultiString(s string) []uint16 {
	// Windows OPENFILENAME filters are NUL-separated pairs ending in a double NUL.
	// syscall.StringToUTF16 rejects embedded NULs, so preserve them manually.
	r := []rune(s)
	u := utf16pkg.Encode(r)
	if len(u) == 0 || u[len(u)-1] != 0 { u = append(u, 0) }
	if len(u) < 2 || u[len(u)-2] != 0 { u = append(u, 0) }
	return u
}

func chooseFile(owner uintptr, filter, title string) string {
	buf := make([]uint16, 4096)
	f := utf16MultiString(filter)
	ofn := OPENFILENAME{
		LStructSize: uint32(unsafe.Sizeof(OPENFILENAME{})),
		HwndOwner: owner,
		LpstrFilter: &f[0],
		LpstrFile: &buf[0],
		NMaxFile: uint32(len(buf)),
		LpstrTitle: utf16(title),
		Flags: OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST,
	}
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 { return "" }
	return syscall.UTF16ToString(buf)
}
