//go:build windows

package desktop

import (
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

// Windows dictation uses the in-process SAPI engine (SpInprocRecognizer).
// That engine stays on this PC. There is no cloud fallback: a language
// without a local recognizer is an error.

func platformEar() Ear { return newWinEar() }

func speechLangHex(locale string) (string, bool) {
	switch locale {
	case "zh-CN":
		return "804", true
	case "en-US":
		return "409", true
	default:
		return "", false
	}
}

type winEar struct {
	mu        sync.Mutex
	listening bool
	text      string
	errCode   int
	startCh   chan startReq
	stopCh    chan stopReq
}

type startReq struct {
	locale string
	done   chan error
}
type stopReq struct{ done chan struct{} }

func newWinEar() *winEar {
	e := &winEar{startCh: make(chan startReq), stopCh: make(chan stopReq)}
	go e.loop()
	return e
}

func (e *winEar) Available() bool {
	_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	_, err := oleutil.ClassIDFrom("SAPI.SpInprocRecognizer")
	return err == nil
}

func (e *winEar) Start(locale string) error {
	done := make(chan error, 1)
	e.startCh <- startReq{locale: locale, done: done}
	select {
	case err := <-done:
		return err
	case <-time.After(60 * time.Second):
		return speechFailure{Code: 5}
	}
}

func (e *winEar) Stop() (string, error) {
	done := make(chan struct{})
	e.stopCh <- stopReq{done: done}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text, nil
}

func (e *winEar) View() SpeechView {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SpeechView{Listening: e.listening, Text: e.text, ErrCode: e.errCode}
}

func (e *winEar) setText(s string) {
	e.mu.Lock()
	e.text = s
	e.mu.Unlock()
}

func (e *winEar) setErr(code int) {
	e.mu.Lock()
	if e.errCode == 0 {
		e.errCode = code
	}
	e.mu.Unlock()
}

func (e *winEar) loop() {
	_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	defer ole.CoUninitialize()
	var session *recoSession
	for {
		if session != nil {
			select {
			case req := <-e.stopCh:
				session.close()
				session = nil
				e.mu.Lock()
				e.listening = false
				e.mu.Unlock()
				close(req.done)
			default:
				pumpMessage()
			}
			continue
		}
		select {
		case req := <-e.startCh:
			e.mu.Lock()
			busy := e.listening
			e.mu.Unlock()
			if busy {
				req.done <- speechFailure{Code: 4}
				continue
			}
			s, err := e.begin(req.locale)
			if err != nil {
				req.done <- err
				continue
			}
			session = s
			e.mu.Lock()
			e.listening = true
			e.text = ""
			e.errCode = 0
			e.mu.Unlock()
			req.done <- nil
		case req := <-e.stopCh:
			close(req.done)
		}
	}
}

func (e *winEar) begin(locale string) (*recoSession, error) {
	lang, ok := speechLangHex(locale)
	if !ok {
		return nil, speechFailure{Code: 3}
	}
	unk, err := oleutil.CreateObject("SAPI.SpInprocRecognizer")
	if err != nil {
		return nil, speechFailure{Code: speechHRESULT(err)}
	}
	disp, err := unk.QueryInterface(ole.IID_IDispatch)
	unk.Release()
	if err != nil {
		return nil, speechFailure{Code: 3}
	}
	reco := (*ole.IDispatch)(unsafe.Pointer(disp))
	token, err := recognizerFor(reco, lang)
	if err != nil {
		reco.Release()
		return nil, err
	}
	if _, err := oleutil.PutProperty(reco, "Recognizer", token); err != nil {
		token.Release()
		reco.Release()
		return nil, speechFailure{Code: speechHRESULT(err)}
	}
	token.Release()
	ctxv, err := oleutil.CallMethod(reco, "CreateRecoContext")
	if err != nil {
		reco.Release()
		return nil, speechFailure{Code: speechHRESULT(err)}
	}
	rctx := ctxv.ToIDispatch()
	sink := newSpeechSink(e)
	point, cookie, err := advise(rctx, sink)
	if err != nil {
		rctx.Release()
		reco.Release()
		return nil, speechFailure{Code: 3}
	}
	const hypothesis, recognition = 32, 16
	if _, err := oleutil.CallMethod(rctx, "SetInterest", int64(hypothesis|recognition), int64(hypothesis|recognition)); err != nil {
		unadvise(point, cookie)
		rctx.Release()
		reco.Release()
		return nil, speechFailure{Code: 3}
	}
	gv, err := oleutil.CallMethod(rctx, "CreateGrammar", 0)
	if err != nil {
		unadvise(point, cookie)
		rctx.Release()
		reco.Release()
		return nil, speechFailure{Code: speechHRESULT(err)}
	}
	grammar := gv.ToIDispatch()
	if _, err := oleutil.CallMethod(grammar, "DictationLoad", "", 0); err != nil {
		grammar.Release()
		unadvise(point, cookie)
		rctx.Release()
		reco.Release()
		return nil, speechFailure{Code: 3}
	}
	if _, err := oleutil.CallMethod(grammar, "DictationSetState", 1); err != nil {
		grammar.Release()
		unadvise(point, cookie)
		rctx.Release()
		reco.Release()
		return nil, speechFailure{Code: speechHRESULT(err)}
	}
	return &recoSession{reco: reco, ctx: rctx, grammar: grammar, point: point, cookie: cookie}, nil
}

func recognizerFor(reco *ole.IDispatch, lang string) (*ole.IDispatch, error) {
	for _, attr := range []string{"Language=" + lang, "Language=" + langDecimal(lang)} {
		v, err := oleutil.CallMethod(reco, "GetRecognizers", attr, "")
		if err != nil || v == nil || v.VT != ole.VT_DISPATCH {
			continue
		}
		tokens := v.ToIDispatch()
		cv, err := oleutil.GetProperty(tokens, "Count")
		n := 0
		if err == nil && cv != nil {
			n = int(cv.Val)
		}
		if n < 1 {
			tokens.Release()
			continue
		}
		item, err := oleutil.CallMethod(tokens, "Item", 0)
		tokens.Release()
		if err != nil || item == nil || item.VT != ole.VT_DISPATCH {
			continue
		}
		return item.ToIDispatch(), nil
	}
	return nil, speechFailure{Code: 3}
}

func langDecimal(hex string) string {
	switch hex {
	case "804":
		return "2052"
	case "409":
		return "1033"
	default:
		return hex
	}
}

func speechHRESULT(err error) int {
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "80070005") || strings.Contains(s, "access") || strings.Contains(s, "denied") {
		return 2
	}
	return 3
}

type recoSession struct {
	reco    *ole.IDispatch
	ctx     *ole.IDispatch
	grammar *ole.IDispatch
	point   *ole.IConnectionPoint
	cookie  uint32
}

func (s *recoSession) close() {
	if s == nil {
		return
	}
	_, _ = oleutil.CallMethod(s.grammar, "DictationSetState", 0)
	unadvise(s.point, s.cookie)
	s.grammar.Release()
	s.ctx.Release()
	s.reco.Release()
	if s.point != nil {
		s.point.Release()
	}
}

var iidRecoEvents = ole.NewGUID("{7B8FCB42-0E9D-4F00-A048-7B04D6179D3D}")

func advise(ctx *ole.IDispatch, sink *speechSink) (*ole.IConnectionPoint, uint32, error) {
	unk, err := ctx.QueryInterface(ole.IID_IConnectionPointContainer)
	if err != nil {
		return nil, 0, err
	}
	container := (*ole.IConnectionPointContainer)(unsafe.Pointer(unk))
	var point *ole.IConnectionPoint
	err = container.FindConnectionPoint(iidRecoEvents, &point)
	container.Release()
	if err != nil {
		return nil, 0, err
	}
	cookie, err := point.Advise((*ole.IUnknown)(unsafe.Pointer(sink)))
	if err != nil {
		point.Release()
		return nil, 0, err
	}
	return point, cookie, nil
}

func unadvise(point *ole.IConnectionPoint, cookie uint32) {
	if point == nil {
		return
	}
	syscall.SyscallN(point.VTable().Unadvise, uintptr(unsafe.Pointer(point)), uintptr(cookie))
}

type speechSink struct {
	lpVtbl *speechSinkVtbl
	ref    int32
	ear    *winEar
}

type speechSinkVtbl struct {
	query   uintptr
	add     uintptr
	release uintptr
	count   uintptr
	info    uintptr
	names   uintptr
	invoke  uintptr
}

var sinkVtbl speechSinkVtbl
var sinkOnce sync.Once

func newSpeechSink(ear *winEar) *speechSink {
	sinkOnce.Do(func() {
		sinkVtbl.query = syscall.NewCallback(sinkQuery)
		sinkVtbl.add = syscall.NewCallback(sinkAdd)
		sinkVtbl.release = syscall.NewCallback(sinkRelease)
		sinkVtbl.count = syscall.NewCallback(sinkCount)
		sinkVtbl.info = syscall.NewCallback(sinkInfo)
		sinkVtbl.names = syscall.NewCallback(sinkNames)
		sinkVtbl.invoke = syscall.NewCallback(sinkInvoke)
	})
	return &speechSink{lpVtbl: &sinkVtbl, ear: ear, ref: 1}
}

func sinkQuery(this *ole.IUnknown, iid *ole.GUID, punk **ole.IUnknown) uintptr {
	s := (*speechSink)(unsafe.Pointer(this))
	*punk = nil
	if ole.IsEqualGUID(iid, ole.IID_IUnknown) || ole.IsEqualGUID(iid, ole.IID_IDispatch) || ole.IsEqualGUID(iid, iidRecoEvents) {
		atomic.AddInt32(&s.ref, 1)
		*punk = this
		return ole.S_OK
	}
	return ole.E_NOINTERFACE
}

func sinkAdd(this *ole.IUnknown) int32 {
	s := (*speechSink)(unsafe.Pointer(this))
	return atomic.AddInt32(&s.ref, 1)
}

func sinkRelease(this *ole.IUnknown) int32 {
	s := (*speechSink)(unsafe.Pointer(this))
	return atomic.AddInt32(&s.ref, -1)
}

func sinkCount(n *int) uintptr {
	if n != nil {
		*n = 0
	}
	return ole.S_OK
}

func sinkInfo() uintptr { return ole.E_NOTIMPL }

func sinkNames() uintptr { return ole.S_OK }

func sinkInvoke(this *ole.IDispatch, dispid int32, riid *ole.GUID, lcid int, flags int16, params *ole.DISPPARAMS, result *ole.VARIANT, excep *ole.EXCEPINFO, nerr *uint) uintptr {
	// Hypothesis is 8, Recognition is 7. The result object is the last argument.
	if dispid != 7 && dispid != 8 {
		return ole.S_OK
	}
	s := (*speechSink)(unsafe.Pointer(this))
	if text := phraseText(params); text != "" && s.ear != nil {
		s.ear.setText(text)
	}
	return ole.S_OK
}

type rawDisp struct {
	rgvarg uintptr
	named  uintptr
	cArgs  uint32
	cNamed uint32
}

func phraseText(params *ole.DISPPARAMS) string {
	if params == nil {
		return ""
	}
	raw := (*rawDisp)(unsafe.Pointer(params))
	if raw.cArgs == 0 || raw.rgvarg == 0 {
		return ""
	}
	v := (*ole.VARIANT)(unsafe.Pointer(raw.rgvarg))
	if v.VT != ole.VT_DISPATCH {
		return ""
	}
	result := v.ToIDispatch()
	if result == nil {
		return ""
	}
	pv, err := oleutil.GetProperty(result, "PhraseInfo")
	if err != nil || pv == nil || pv.VT != ole.VT_DISPATCH {
		return ""
	}
	phrase := pv.ToIDispatch()
	tv, err := oleutil.CallMethod(phrase, "GetText")
	if err != nil || tv == nil {
		return ""
	}
	return tv.ToString()
}

var (
	user32      = windows.NewLazySystemDLL("user32.dll")
	peekMessage = user32.NewProc("PeekMessageW")
	translate   = user32.NewProc("TranslateMessage")
	dispatchMsg = user32.NewProc("DispatchMessageW")
)

type winMSG struct {
	hwnd    uintptr
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

func pumpMessage() {
	var m winMSG
	r, _, _ := peekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
	if r == 0 {
		time.Sleep(20 * time.Millisecond)
		return
	}
	translate.Call(uintptr(unsafe.Pointer(&m)))
	dispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
}
