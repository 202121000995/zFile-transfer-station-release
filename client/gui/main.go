package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var u32 = syscall.NewLazyDLL("user32.dll")
var k32 = syscall.NewLazyDLL("kernel32.dll")
var cd32 = syscall.NewLazyDLL("comdlg32.dll")
var s32 = syscall.NewLazyDLL("shell32.dll")
var g32 = syscall.NewLazyDLL("gdi32.dll")

func u16(s string) *uint16                         { p, _ := syscall.UTF16PtrFromString(s); return p }
func u16s(b []uint16) string                       { return syscall.UTF16ToString(b) }
func cl(p *syscall.LazyProc, a ...uintptr) uintptr { r, _, _ := p.Call(a...); return r }

var (
	rReg   = u32.NewProc("RegisterClassExW")
	rCW    = u32.NewProc("CreateWindowExW")
	rDP    = u32.NewProc("DefWindowProcW")
	rGM    = u32.NewProc("GetMessageW")
	rTM    = u32.NewProc("TranslateMessage")
	rDM    = u32.NewProc("DispatchMessageW")
	rSM    = u32.NewProc("SendMessageW")
	rST    = u32.NewProc("SetWindowTextW")
	rGTL   = u32.NewProc("GetWindowTextLengthW")
	rGT    = u32.NewProc("GetWindowTextW")
	rGD    = u32.NewProc("GetDlgItem")
	rSW    = u32.NewProc("ShowWindow")
	rPQ    = u32.NewProc("PostQuitMessage")
	rBP    = u32.NewProc("BeginPaint")
	rEP    = u32.NewProc("EndPaint")
	rFR    = u32.NewProc("FillRect")
	rDT    = u32.NewProc("DrawTextW")
	rIR    = u32.NewProc("InvalidateRect")
	rLC    = u32.NewProc("LoadCursorW")
	rSC    = u32.NewProc("SetCursor")
	rAWR   = u32.NewProc("AdjustWindowRect")
	rGOF   = cd32.NewProc("GetOpenFileNameW")
	rGMH   = k32.NewProc("GetModuleHandleW")
	rGA    = k32.NewProc("GlobalAlloc")
	rGL    = k32.NewProc("GlobalLock")
	rGU    = k32.NewProc("GlobalUnlock")
	rOC    = u32.NewProc("OpenClipboard")
	rEC    = u32.NewProc("EmptyClipboard")
	rSCL   = u32.NewProc("SetClipboardData")
	rCC    = u32.NewProc("CloseClipboard")
	rDA    = s32.NewProc("DragAcceptFiles")
	rDQ    = s32.NewProc("DragQueryFileW")
	rDF    = s32.NewProc("DragFinish")
	rMB    = u32.NewProc("MessageBoxW")
	rCF    = g32.NewProc("CreateFontW")
	rCB    = g32.NewProc("CreateSolidBrush")
	rCP    = g32.NewProc("CreatePen")
	rSO    = g32.NewProc("SelectObject")
	rRR    = g32.NewProc("RoundRect")
	rSBM   = g32.NewProc("SetBkMode")
	rSTC   = g32.NewProc("SetTextColor")
	rMT    = g32.NewProc("MoveToEx")
	rLT    = g32.NewProc("LineTo")
	rSDC   = g32.NewProc("SaveDC")
	rRDC   = g32.NewProc("RestoreDC")
	rGS    = g32.NewProc("GetStockObject")
	rCDC   = g32.NewProc("CreateCompatibleDC")
	rCBMP  = g32.NewProc("CreateCompatibleBitmap")
	rBB    = g32.NewProc("BitBlt")
	rDelDC = g32.NewProc("DeleteDC")
	rDelO  = g32.NewProc("DeleteObject")
)

const (
	WP      = 0x80000000
	WV      = 0x10000000
	WCAP    = 0x00C00000
	WSYS    = 0x00080000
	WMIN    = 0x00020000
	WC      = 0x40000000
	WT      = 0x00010000
	WA      = 0x00000010
	EC      = 0x0001
	WMP     = 0x000F
	WEB     = 0x0014
	WMD     = 0x0002
	WDF     = 0x0233
	WLD     = 0x0201
	WMM     = 0x0200
	WSF     = 0x0030
	WSC     = 0x0020
	WNCH    = 0x0084
	WNCL    = 0x00A1
	HT      = 2
	HC      = 1
	DTC     = 0x01
	DVC     = 0x04
	DSL     = 0x20
	GM      = 0x0002
	CF      = 1
	SWM     = 6
	N1      = ^uintptr(0)
	NP      = 8
	NB      = 5
	IDA     = 32512
	IDH     = 32649
	SRCCOPY = 0x00CC0020
)

// v3.0 Colors (HEX -> 0x00BBGGRR)
const (
	CBG  = 0xFBFAF9 // #F9FAFB bg
	CWH  = 0xFFFFFF // #FFFFFF white
	CPR  = 0xF67C2F // #2F7CF6 primary
	CPRH = 0xF26E1E // #1E6EF2 primary hover
	CTT  = 0x271811 // #111827 text title
	CTB  = 0x514137 // #374151 text body
	CTH  = 0x80726B // #6B7280 text hint
	CBD  = 0xDBD5D1 // #D1D5DB border
	CDV  = 0xEBE7E5 // #E5E7EB divider
	CSU  = 0xFFF6EF // #EFF6FF success bg
	CHV  = 0xFFF8F4 // #F4F8FF hover bg
	CDG  = 0x4444EF // #EF4444 danger
	CSCB = 0xFEDBBF // #BFDBFE card border
	CST  = 0xAF401E // #1E40AF success title
	CCD  = 0xEB6325 // #2563EB code text
	CDB  = 0xFF9D5B // #5B9DFF dash border (dash=5B9DFF -> FBC89D... wait)
)

// v3.0 Layout
const (
	WW  = 420
	PX  = 24
	CW  = WW - 2*PX
	TB  = 0
	TH  = 88
	FH  = 44
	CY  = TB + TH
	CH  = 385
	WH  = TB + TH + CH + FH
	CID = 201
)

const maxUploadBytes int64 = 500 * 1024 * 1024

var (
	hI, hM                                        uintptr
	tab                                           int
	sURL, sPath                                   string
	uping, dling                                  bool
	upCode                                        string
	upPct, dlPct                                  int
	dlMsg                                         string
	hOn                                           int
	backDC, backBmp, backBmpOld                   uintptr
	fTitle, fBody, fBtn, fSmall, fCode, fUp       uintptr
	bBG, bWHT, bPR, bPRH, bSU, bHV, bCB, bST, bCD uintptr
	pBD, pDV, pPR                                 uintptr
	hArr, hHnd                                    uintptr
)
var logf *os.File

func log(s string) {
	if logf == nil {
		logf, _ = os.Create("D:\\codex\\wenjianzhongzhuan\\build\\client\\transfer.log")
	}
	if logf != nil {
		logf.WriteString(s + "\r\n")
		logf.Sync()
	}
}
func mf(h int, w int) uintptr {
	return cl(rCF, uintptr(h), 0, 0, 0, uintptr(w), 0, 0, 0, 1, 0, 0, 0, 0, uintptr(unsafe.Pointer(u16("Segoe UI"))))
}
func lcfg() string {
	d, _ := os.ReadFile("transfer-config.txt")
	s := strings.TrimSpace(string(d))
	if s != "" {
		return strings.TrimRight(s, "/")
	}
	return "https://zz.31pk.top"
}
func initGDI() {
	fTitle = mf(-14, 700)
	fBody = mf(-14, 400)
	fBtn = mf(-14, 600)
	fSmall = mf(-12, 400)
	fCode = mf(-36, 700)
	fUp = mf(-16, 600)
	bBG = cl(rCB, CBG)
	bWHT = cl(rCB, CWH)
	bPR = cl(rCB, CPR)
	bPRH = cl(rCB, CPRH)
	bSU = cl(rCB, CSU)
	bHV = cl(rCB, CHV)
	bCB = cl(rCB, CSCB)
	bST = cl(rCB, CST)
	bCD = cl(rCB, CCD)
	pBD = cl(rCP, 0, 1, CBD)
	pDV = cl(rCP, 0, 1, CDV)
	pPR = cl(rCP, 0, 1, CPR)
	hArr = cl(rLC, 0, IDA)
	hHnd = cl(rLC, 0, IDH)
}
func initBackBuf(hdc uintptr) {
	backDC = cl(rCDC, hdc)
	backBmp = cl(rCBMP, hdc, WW, WH)
	backBmpOld = cl(rSO, backDC, backBmp)
}
func freeBackBuf() {
	if backBmpOld != 0 {
		cl(rSO, backDC, backBmpOld)
	}
	if backBmp != 0 {
		cl(rDelO, backBmp)
	}
	if backDC != 0 {
		cl(rDelDC, backDC)
	}
}

type Rc struct{ L, T, R, B int32 }

func SF(hdc uintptr, x, y, w, h int32, br uintptr) {
	cl(rSDC, hdc)
	cl(rSO, hdc, cl(rGS, NP))
	cl(rSO, hdc, br)
	r := Rc{x, y, x + w, y + h}
	cl(rFR, hdc, uintptr(unsafe.Pointer(&r)), br)
	cl(rRDC, hdc, N1)
}
func SR(hdc uintptr, x, y, w, h, rad int32, pen, br uintptr) {
	cl(rSDC, hdc)
	if pen != 0 {
		cl(rSO, hdc, pen)
	} else {
		cl(rSO, hdc, cl(rGS, NP))
	}
	if br != 0 {
		cl(rSO, hdc, br)
	} else {
		cl(rSO, hdc, cl(rGS, NB))
	}
	cl(rRR, hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), uintptr(rad), uintptr(rad))
	cl(rRDC, hdc, N1)
}
func ST(hdc uintptr, s string, x, y, w, h int32, fmt int, color uintptr) {
	cl(rSDC, hdc)
	cl(rSBM, hdc, 1)
	cl(rSTC, hdc, color)
	r := Rc{x, y, x + w, y + h}
	cl(rDT, hdc, uintptr(unsafe.Pointer(u16(s))), N1, uintptr(unsafe.Pointer(&r)), uintptr(fmt))
	cl(rRDC, hdc, N1)
}
func SL(hdc uintptr, x1, y1, x2, y2 int32, pen uintptr) {
	cl(rSDC, hdc)
	cl(rSO, hdc, pen)
	cl(rMT, hdc, uintptr(x1), uintptr(y1), 0)
	cl(rLT, hdc, uintptr(x2), uintptr(y2))
	cl(rRDC, hdc, N1)
}
func SDH(hdc uintptr, x, y, w int32, pen uintptr) {
	cl(rSDC, hdc)
	cl(rSO, hdc, pen)
	for i := int32(0); i < w; i += 8 {
		e := i + 4
		if e > w {
			e = w
		}
		cl(rMT, hdc, uintptr(x+i), uintptr(y), 0)
		cl(rLT, hdc, uintptr(x+e), uintptr(y))
	}
	cl(rRDC, hdc, N1)
}
func SDV(hdc uintptr, x, y, h int32, pen uintptr) {
	cl(rSDC, hdc)
	cl(rSO, hdc, pen)
	for i := int32(0); i < h; i += 8 {
		e := i + 4
		if e > h {
			e = h
		}
		cl(rMT, hdc, uintptr(x), uintptr(y+i), 0)
		cl(rLT, hdc, uintptr(x), uintptr(y+e))
	}
	cl(rRDC, hdc, N1)
}
func redraw() { cl(rIR, hM, 0, 1) }

func iconText(hdc uintptr, s string, x, y, w, h int32, font, color uintptr) {
	cl(rSO, hdc, font)
	ST(hdc, s, x, y, w, h, DTC|DVC|DSL, color)
}

func drawUploadBadge(hdc uintptr, x, y int32, color uintptr, size int32) {
	r := size / 2
	SR(hdc, x+size/5, y+size/3, size*3/5, size/2, r/2, 0, color)
	cl(rSDC, hdc)
	cl(rSO, hdc, cl(rGS, NP))
	cl(rSO, hdc, color)
	cl(g32.NewProc("Ellipse"), hdc, uintptr(x+size/8), uintptr(y+size/3), uintptr(x+size/2), uintptr(y+size*3/4))
	cl(g32.NewProc("Ellipse"), hdc, uintptr(x+size/3), uintptr(y+size/6), uintptr(x+size*3/4), uintptr(y+size*2/3))
	cl(g32.NewProc("Ellipse"), hdc, uintptr(x+size/2), uintptr(y+size/3), uintptr(x+size*7/8), uintptr(y+size*3/4))
	cl(rRDC, hdc, N1)
	iconText(hdc, "\u2191", x, y+size/7, size, size, fCode, CWH)
}

func drawInboxBadge(hdc uintptr, x, y int32, color uintptr) {
	SR(hdc, x+10, y+18, 44, 30, 7, 0, color)
	SL(hdc, x+17, y+33, x+28, y+33, cl(rCP, 0, 3, CWH))
	SL(hdc, x+28, y+33, x+33, y+39, cl(rCP, 0, 3, CWH))
	SL(hdc, x+33, y+39, x+47, y+39, cl(rCP, 0, 3, CWH))
}

func drawSmallFile(hdc uintptr, x, y int32, color uintptr) {
	SR(hdc, x, y, 18, 22, 2, cl(rCP, 0, 1, color), 0)
	SL(hdc, x+4, y+8, x+14, y+8, cl(rCP, 0, 1, color))
	SL(hdc, x+4, y+13, x+14, y+13, cl(rCP, 0, 1, color))
}

func drawCheckBox(hdc uintptr, x, y int32, checked bool) {
	if checked {
		SR(hdc, x, y, 16, 16, 4, 0, bPR)
		iconText(hdc, "\u2713", x, y-1, 16, 16, fSmall, CWH)
		return
	}
	SR(hdc, x, y, 16, 16, 4, pBD, bWHT)
}

func paintAll(hdc uintptr) {
	if hM == 0 {
		return
	}
	dc := hdc
	SF(dc, 0, 0, WW, WH, bBG)

	// Tab bar
	tabBottom := int32(CY)
	txY := int32(17)
	sc := uintptr(CTH)
	rc := uintptr(CTH)
	if tab == 0 {
		sc = uintptr(CPR)
	} else {
		rc = uintptr(CPR)
	}
	iconText(dc, "\u21e7", 58, txY-2, 28, 32, fCode, sc)
	cl(rSO, dc, fTitle)
	ST(dc, "\u53d1\u9001\u6587\u4ef6", 88, txY, 100, 32, DTC|DVC|DSL, sc)
	iconText(dc, "\u21e9", 236, txY-2, 28, 32, fCode, rc)
	cl(rSO, dc, fTitle)
	ST(dc, "\u63a5\u6536\u6587\u4ef6", 266, txY, 100, 32, DTC|DVC|DSL, rc)
	if tab == 0 {
		SF(dc, 24, tabBottom-4, 178, 3, bPR)
	} else {
		SF(dc, 218, tabBottom-4, 178, 3, bPR)
	}
	SL(dc, 0, tabBottom-1, WW, tabBottom-1, pDV)

	cy := int32(CY)
	if tab == 0 {
		dzX := int32(24)
		dzY := cy + 20
		dzW := int32(372)
		dzH := int32(150)
		dzBg := bWHT
		if hOn == 7 {
			dzBg = bHV
		}
		SR(dc, dzX, dzY, dzW, dzH, 12, 0, dzBg)
		SDH(dc, dzX, dzY, dzW, pPR)
		SDH(dc, dzX, dzY+dzH-1, dzW, pPR)
		SDV(dc, dzX, dzY, dzH, pPR)
		SDV(dc, dzX+dzW-1, dzY, dzH, pPR)
		drawUploadBadge(dc, dzX+(dzW-68)/2, dzY+26, CPR, 68)
		cl(rSO, dc, fUp)
		ST(dc, "\u70b9\u51fb\u6216\u62d6\u62fd\u6587\u4ef6\u5230\u6b64\u5904\u4e0a\u4f20", dzX, dzY+88, dzW, 26, DTC|DSL, CTT)
		cl(rSO, dc, fSmall)
		ST(dc, "\u652f\u6301\u5355\u4e2a\u6216\u591a\u4e2a\u6587\u4ef6\uff08\u6700\u5927 500MB\uff09", dzX, dzY+118, dzW, 20, DTC|DSL, CTH)
		if sPath != "" {
			ST(dc, "\U0001F4C4 "+filepath.Base(sPath), dzX, dzY+136, dzW, 18, DTC|DSL, CTH)
		}

		btnY := int32(275)
		btnH := int32(42)
		btnBr := bPR
		if hOn == 3 && !uping {
			btnBr = bPRH
		}
		SR(dc, dzX, btnY, dzW, btnH, 6, 0, btnBr)
		iconText(dc, "\u2601", dzX+118, btnY, 34, btnH, fUp, CWH)
		cl(rSO, dc, fBtn)
		cl(rSDC, dc)
		cl(rSBM, dc, 1)
		cl(rSTC, dc, CWH)
		rBtn := Rc{dzX + 30, btnY, dzX + dzW, btnY + btnH}
		cl(rDT, dc, uintptr(unsafe.Pointer(u16("\u5f00\u59cb\u4e0a\u4f20\u6587\u4ef6"))), N1, uintptr(unsafe.Pointer(&rBtn)), uintptr(DTC|DVC|DSL))
		cl(rRDC, dc, N1)

		if uping || upPct > 0 {
			progY := int32(330)
			progH := int32(6)
			SF(dc, dzX, progY, dzW, progH, cl(rCB, CDV))
			if upPct > 0 {
				fw := int32(upPct) * dzW / 100
				if fw < 6 {
					fw = 6
				}
				SF(dc, dzX, progY, fw, progH, bPR)
			}
		}

		if upCode != "" {
			cardY := int32(350)
			cardH := int32(96)
			SR(dc, dzX, cardY, dzW, cardH, 8, cl(rCP, 0, 1, CSCB), bSU)
			drawSmallFile(dc, 42, cardY+15, CPR)
			cl(rSO, dc, fBtn)
			ST(dc, "\u4e0a\u4f20\u7ed3\u679c", 68, cardY+12, 160, 22, DSL, CTB)
			cl(rSO, dc, fSmall)
			ST(dc, "\u4e0a\u4f20\u6210\u529f\uff01\u60a8\u7684\u53d6\u4ef6\u7801\u662f\uff1a", 78, cardY+34, 210, 20, DSL, CTT)
			cl(rSO, dc, fCode)
			ST(dc, upCode, 96, cardY+52, 180, 40, DVC|DSL, CPR)
			cl(rSO, dc, fSmall)
			ST(dc, "\u6587\u4ef6\u5c06\u5728 24 \u5c0f\u65f6\u540e\u81ea\u52a8\u8fc7\u671f", 96, cardY+82, 210, 18, DSL, CTH)
			cpX := int32(296)
			cpY := cardY + 48
			cpW := int32(72)
			cpH := int32(30)
			brCp := bWHT
			tcCp := uintptr(CTH)
			if hOn == 8 {
				brCp = bSU
				tcCp = uintptr(CPR)
			}
			SR(dc, cpX, cpY, cpW, cpH, 4, cl(rCP, 0, 1, CSCB), brCp)
			drawSmallFile(dc, cpX+9, cpY+5, CPR)
			cl(rSO, dc, fSmall)
			cl(rSDC, dc)
			cl(rSBM, dc, 1)
			cl(rSTC, dc, tcCp)
			rCp := Rc{cpX + 22, cpY, cpX + cpW, cpY + cpH}
			cl(rDT, dc, uintptr(unsafe.Pointer(u16("\u590d\u5236"))), N1, uintptr(unsafe.Pointer(&rCp)), uintptr(DTC|DVC|DSL))
			cl(rRDC, dc, N1)
		}
	} else {
		cl(rSO, dc, fTitle)
		ST(dc, "\u8bf7\u8f93\u5165\u53d6\u4ef6\u7801", 24, cy+30, 200, 28, DSL, CTT)
		inpY := cy + 62
		inpH := int32(40)
		SR(dc, 24, inpY, 250, inpH, 6, pBD, bWHT)
		btnX := int32(290)
		btnW := int32(106)
		btnBr := bPR
		if hOn == 5 && !dling {
			btnBr = bPRH
		}
		SR(dc, btnX, inpY, btnW, inpH, 6, 0, btnBr)
		iconText(dc, "\u21e9", btnX+12, inpY, 26, inpH, fUp, CWH)
		cl(rSO, dc, fBtn)
		cl(rSDC, dc)
		cl(rSBM, dc, 1)
		cl(rSTC, dc, CWH)
		rDl := Rc{btnX + 22, inpY, btnX + btnW, inpY + inpH}
		cl(rDT, dc, uintptr(unsafe.Pointer(u16("\u63d0\u53d6\u4e0b\u8f7d"))), N1, uintptr(unsafe.Pointer(&rDl)), uintptr(DTC|DVC|DSL))
		cl(rRDC, dc, N1)

		if dling || dlPct > 0 {
			progY := inpY + inpH + 10
			progH := int32(6)
			SF(dc, 24, progY, CW, progH, cl(rCB, CDV))
			if dlPct > 0 {
				fw := int32(dlPct) * CW / 100
				if fw < 6 {
					fw = 6
				}
				SF(dc, 24, progY, fw, progH, bPR)
			}
		}

		statY := cy + 160
		statH := int32(130)
		SR(dc, 24, statY, CW, statH, 8, pBD, bWHT)
		stxt := dlMsg
		if stxt == "" {
			stxt = "\u7b49\u5f85\u8f93\u5165\u53d6\u4ef6\u7801..."
		}
		stC := uintptr(CTH)
		if strings.Contains(stxt, "\u5b8c\u6210") || strings.Contains(stxt, "Done") {
			stC = uintptr(CPR)
		}
		drawInboxBadge(dc, 178, statY+28, CTH)
		cl(rSO, dc, fUp)
		ST(dc, stxt, 24, statY+76, CW, 28, DTC|DSL, stC)
		cl(rSO, dc, fSmall)
		ST(dc, "\u8f93\u5165\u6709\u6548\u7684\u53d6\u4ef6\u7801\u5373\u53ef\u5f00\u59cb\u4e0b\u8f7d\u6587\u4ef6", 24, statY+106, CW, 20, DTC|DSL, CTH)
		chkY1 := cy + 315
		drawCheckBox(dc, 24, chkY1, true)
		ST(dc, "\u4e0b\u8f7d\u5b8c\u6210\u540e\u81ea\u52a8\u6253\u5f00\u6587\u4ef6\u5939", 48, chkY1, 300, 18, DVC|DSL, CTB)
		chkY2 := cy + 350
		drawCheckBox(dc, 24, chkY2, true)
		ST(dc, "\u4e0b\u8f7d\u7684\u6587\u4ef6\u4e3a\u538b\u7f29\u5305\u5219\u81ea\u52a8\u89e3\u538b", 48, chkY2, 340, 18, DVC|DSL, CTB)
	}

	fy := int32(CY + CH)
	SF(dc, 0, fy, WW, FH, bBG)
	SL(dc, 0, fy, WW, fy, pDV)
	cl(rSO, dc, fSmall)
	drawCheckBox(dc, 24, fy+12, false)
	ST(dc, "\u5f00\u673a\u81ea\u52a8\u542f\u52a8", 48, fy+10, 140, 20, DVC|DSL, CTB)
	stX := int32(300)
	stY := fy + 7
	stW := int32(88)
	stH := int32(30)
	brSt := bWHT
	if hOn == 9 {
		brSt = bHV
	}
	SR(dc, stX, stY, stW, stH, 6, pBD, brSt)
	iconText(dc, "\u2699", stX+8, stY, 22, stH, fSmall, CTB)
	ST(dc, "\u8bbe\u7f6e\u4e2d\u5fc3", stX+24, stY, stW-24, stH, DVC|DSL, CTB)
}
func ht(mx, my int32) int {
	if my < int32(CY) {
		if mx >= 24 && mx <= 202 {
			return 1
		}
		if mx >= 218 && mx <= 396 {
			return 2
		}
		return 0
	}
	if tab == 0 {
		if mx >= 24 && mx <= 396 && my >= 108 && my <= 258 {
			return 7
		}
		if mx >= 24 && mx <= 396 && my >= 275 && my <= 317 && !uping {
			return 3
		}
		if upCode != "" {
			if mx >= 296 && mx <= 368 && my >= 398 && my <= 428 {
				return 8
			}
		}
	} else {
		if mx >= 290 && mx <= 396 && my >= 150 && my <= 190 && !dling {
			return 5
		}
	}
	fy := int32(CY + CH)
	if my >= fy {
		if mx >= 300 && mx <= 388 && my >= fy+7 && my <= fy+37 {
			return 9
		}
	}
	return 0
}
func getET(id int) string {
	hwnd := cl(rGD, hM, uintptr(id))
	if hwnd == 0 {
		return ""
	}
	l := int(cl(rGTL, hwnd))
	if l <= 0 {
		return ""
	}
	buf := make([]uint16, l+1)
	cl(rGT, hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(l+1))
	return u16s(buf)
}
func setupEdits(parent uintptr) {
	inpY := int32(CY + 62)
	inpH := int32(40)
	cl(rCW, 0, uintptr(unsafe.Pointer(u16("EDIT"))), 0, WC|EC|0x00000080,
		uintptr(36), uintptr(inpY+8), uintptr(230), uintptr(inpH-12), parent, CID, hM, 0)
}
func showEdits() { cl(rSW, cl(rGD, hM, CID), 1) }
func pickFile() {
	var buf [520]uint16
	ofl := [88]uintptr{88, hM, 0, uintptr(unsafe.Pointer(u16("All Files(*.*)\000*.*\000"))), 0, 0, 1, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0, 0, 0, 0x80000 | 0x1000, 0, 0}
	cl(rGOF, uintptr(unsafe.Pointer(&ofl[0])))
	sPath = u16s(buf[:])
	if sPath != "" {
		redraw()
	}
}
func handleDrop(wp uintptr) {
	var buf [520]uint16
	cl(rDQ, wp, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	sPath = u16s(buf[:])
	cl(rDF, wp)
	redraw()
}
func copyCode() {
	cl(rOC, 0)
	cl(rEC, 0)
	p, _ := syscall.UTF16PtrFromString(upCode)
	cl(rSCL, CF, uintptr(unsafe.Pointer(p)))
	cl(rCC)
}
func showAbout() {
	cl(rMB, hM, uintptr(unsafe.Pointer(u16("\u8bbe\u7f6e"))), uintptr(unsafe.Pointer(u16("\u8bbe\u7f6e\u4e2d\u5fc3\u529f\u80fd\u5f85\u5b9e\u73b0"))), 0x40)
}
func doUpload() {
	if sPath == "" {
		pickFile()
	}
	if sPath == "" {
		return
	}
	if uping {
		return
	}
	uping = true
	upPct = 0
	upCode = ""
	redraw()
	go func() {
		defer func() { uping = false; redraw() }()
		f, err := os.Open(sPath)
		if err != nil {
			cl(rMB, hM, uintptr(unsafe.Pointer(u16("Error"))), uintptr(unsafe.Pointer(u16("\u65e0\u6cd5\u6253\u5f00\u6587\u4ef6"))), 0x10)
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		total := fi.Size()
		if total > maxUploadBytes {
			cl(rMB, hM, uintptr(unsafe.Pointer(u16("Error"))), uintptr(unsafe.Pointer(u16("\u6587\u4ef6\u8d85\u8fc7\u4e0a\u4f20\u9650\u5236\uff0c\u6700\u5927\u5141\u8bb8 500MB"))), 0x10)
			return
		}
		pr := &progressReader{r: f, total: total, cb: func(n int64) {
			pct := int(n * 100 / total)
			if pct != upPct {
				upPct = pct
				redraw()
			}
		}}
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		pw, _ := w.CreateFormFile("file", filepath.Base(sPath))
		io.Copy(pw, pr)
		w.Close()
		req, _ := http.NewRequest("POST", sURL+"/upload", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		resp, err := (&http.Client{Timeout: 3600 * time.Second}).Do(req)
		if err != nil || resp.StatusCode != 200 {
			cl(rMB, hM, uintptr(unsafe.Pointer(u16("Error"))), uintptr(unsafe.Pointer(u16("\u4e0a\u4f20\u5931\u8d25"))), 0x10)
			return
		}
		defer resp.Body.Close()
		bd, _ := io.ReadAll(resp.Body)
		upCode = strings.TrimSpace(string(bd))
		redraw()
	}()
}

type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	cb    func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	p.cb(p.read)
	return n, err
}
func doDownload() {
	if dling {
		return
	}
	code := strings.TrimSpace(getET(CID))
	if len(code) != 5 {
		cl(rMB, hM, uintptr(unsafe.Pointer(u16("Error"))), uintptr(unsafe.Pointer(u16("\u8bf7\u8f93\u51655\u4f4d\u53d6\u4ef6\u7801"))), 0x10)
		return
	}
	dling = true
	dlPct = 0
	dlMsg = "\u6b63\u5728\u8fde\u63a5..."
	redraw()
	go func() {
		defer func() { dling = false; redraw() }()
		hc := &http.Client{Timeout: 3600 * time.Second}
		hr, _ := http.NewRequest("HEAD", sURL+"/download/"+code, nil)
		resp, err := hc.Do(hr)
		if err != nil || (resp.StatusCode != 200 && resp.StatusCode != 206) {
			dlMsg = "\u53d6\u4ef6\u7801\u4e0d\u5b58\u5728"
			return
		}
		total, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		fname := resp.Header.Get("X-File-Name")
		if fname == "" {
			fname = code
		}
		resp.Body.Close()
		dlMsg = "\u4e0b\u8f7d\u4e2d: " + fname
		req, _ := http.NewRequest("GET", sURL+"/download/"+code, nil)
		resp2, err := hc.Do(req)
		if err != nil || (resp2.StatusCode != 200 && resp2.StatusCode != 206) {
			dlMsg = "\u4e0b\u8f7d\u5931\u8d25"
			return
		}
		defer resp2.Body.Close()
		out, _ := os.Create(fname)
		defer out.Close()
		var rd int64
		buf2 := make([]byte, 32768)
		for {
			if total > 0 {
				pct := int(rd * 100 / total)
				if pct != dlPct {
					dlPct = pct
					redraw()
				}
			}
			n, err := resp2.Body.Read(buf2)
			if n > 0 {
				out.Write(buf2[:n])
				rd += int64(n)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				dlMsg = "\u4e0b\u8f7d\u9519\u8bef"
				return
			}
		}
		dlMsg = "\u5b8c\u6210: " + fname
	}()
}
func wndProc(hwnd uintptr, msg uint32, wp uintptr, lp uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			f, _ := os.Create("D:\\codex\\wenjianzhongzhuan\\build\\client\\crash.log")
			if f != nil {
				fmt.Fprintf(f, "PANIC msg=0x%X hwnd=%d\n%v\n", msg, hwnd, r)
				f.Close()
			}
			cl(rPQ, 0)
		}
	}()
	switch msg {
	case 0x0001:
		log("WM_CREATE")
		initGDI()
		log("GDI init done")
		sURL = lcfg()
		log("config: " + sURL)
		// Get DC for back buffer init
		// backbuf disabled
		setupEdits(hwnd)
		log("edits created")
		cl(rDA, hwnd, 1)
	case 0x000F:
		var ps [64]byte
		hdc := cl(rBP, hwnd, uintptr(unsafe.Pointer(&ps[0])))
		paintAll(hdc)
		cl(rEP, hwnd, uintptr(unsafe.Pointer(&ps[0])))
		return 0
	case 0x0202:
		pt := lp
		mx := int32(uint16(pt))
		my := int32(uint16(pt >> 16))
		hid := ht(mx, my)
		switch hid {
		case 10:
			cl(rSW, hwnd, 6)
		case 11:
			cl(rSM, hwnd, 0x0010, 0, 0)
		case 1:
			tab = 0
			cl(rSW, cl(rGD, hM, CID), 0)
			redraw()
		case 2:
			tab = 1
			cl(rSW, cl(rGD, hM, CID), 1)
			redraw()
		case 7:
			pickFile()
		case 3:
			doUpload()
		case 8:
			copyCode()
		case 5:
			doDownload()
		case 9:
			showAbout()
		}
	case 0x0200:
		pt := lp
		mx := int32(uint16(pt))
		my := int32(uint16(pt >> 16))
		hid := ht(mx, my)
		if hid != hOn {
			hOn = hid
			redraw()
		}
		var tme [16]byte
		*(*uint32)(unsafe.Pointer(&tme[0])) = 16
		*(*uint32)(unsafe.Pointer(&tme[4])) = 2
		*(*uintptr)(unsafe.Pointer(&tme[8])) = hwnd
		cl(u32.NewProc("TrackMouseEvent"), uintptr(unsafe.Pointer(&tme[0])))
	case 0x02A3:
		hOn = 0
		redraw()
	case 0x0233:
		if uintptr(uint16(wp>>16)) == 0x0300 && uintptr(uint16(wp)) == CID {
			redraw()
		}
	case 0x0020:
		cl(rSC, hArr)
		return 1
	case 0x0133:
		cl(rSO, uintptr(wp), bWHT)
		return uintptr(bWHT)
	case 0x0014:
		return 1
	case 0x0010:
		cl(rPQ, 0)
		return 0
	case 0x0002:
		freeBackBuf()
		cl(rPQ, 0)
	}
	return cl(rDP, hwnd, uintptr(msg), wp, lp)
}
func main() {
	log("=== START v3.0 ===")
	sURL = lcfg()
	log("config: " + sURL)
	hI = cl(rGMH, 0)
	log("hI=" + fmt.Sprint(hI))
	var wc [48]byte
	*(*uint32)(unsafe.Pointer(&wc[0])) = 48
	*(*uint32)(unsafe.Pointer(&wc[4])) = 3
	*(*uintptr)(unsafe.Pointer(&wc[8])) = syscall.NewCallback(wndProc)
	*(*uintptr)(unsafe.Pointer(&wc[20])) = hI
	*(*uintptr)(unsafe.Pointer(&wc[28])) = hArr
	*(*uintptr)(unsafe.Pointer(&wc[32])) = bBG
	*(*uintptr)(unsafe.Pointer(&wc[40])) = uintptr(unsafe.Pointer(u16("TransferV3")))
	cl(rReg, uintptr(unsafe.Pointer(&wc[0])))
	log("class registered")
	screenW := int(cl(u32.NewProc("GetSystemMetrics"), 0))
	screenH := int(cl(u32.NewProc("GetSystemMetrics"), 1))
	x := (screenW - WW) / 2
	if x < 0 {
		x = 0
	}
	y := (screenH - WH) / 2
	if y < 0 {
		y = 0
	}
	style := uintptr(WCAP | WSYS | WMIN | WV)
	var wr [16]byte
	*(*int32)(unsafe.Pointer(&wr[0])) = 0
	*(*int32)(unsafe.Pointer(&wr[4])) = 0
	*(*int32)(unsafe.Pointer(&wr[8])) = WW
	*(*int32)(unsafe.Pointer(&wr[12])) = WH
	cl(rAWR, uintptr(unsafe.Pointer(&wr[0])), style, 0)
	winW := *(*int32)(unsafe.Pointer(&wr[8])) - *(*int32)(unsafe.Pointer(&wr[0]))
	winH := *(*int32)(unsafe.Pointer(&wr[12])) - *(*int32)(unsafe.Pointer(&wr[4]))
	hM = cl(rCW, 0, uintptr(unsafe.Pointer(u16("TransferV3"))),
		uintptr(unsafe.Pointer(u16("\u6587\u4ef6\u4e2d\u8f6c\u7ad9 v1.0"))),
		style, uintptr(x), uintptr(y), uintptr(winW), uintptr(winH), 0, 0, hI, 0)
	if hM == 0 {
		return
	}
	cl(rSW, hM, 5)
	log("window shown")
	var msg [28]byte
	for cl(rGM, uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0) != 0 {
		cl(rTM, uintptr(unsafe.Pointer(&msg[0])))
		cl(rDM, uintptr(unsafe.Pointer(&msg[0])))
	}
}
