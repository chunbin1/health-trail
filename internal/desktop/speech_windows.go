//go:build windows

package desktop

import (
	"runtime"
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
//
// COM objects live in one apartment: every SAPI call, and the message pump
// that delivers its events, runs on one locked OS thread (loop).

func platformEar() Ear { return &winEar{} }

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

// How long a "no recognizer here" answer is kept before asking SAPI again
// (the person may install a speech language meanwhile).
const availableFor = time.Minute

type winEar struct {
	once sync.Once
	reqs chan func(*earThread)

	mu        sync.Mutex
	listening bool
	final     string // phrases recognized so far
	hyp       string // the phrase still being heard
	sep       string // between phrases: a space in English, nothing in Chinese
	errCode   int
}

// do runs f on the COM thread and waits for it.
func (e *winEar) do(f func(*earThread)) {
	e.once.Do(func() {
		e.reqs = make(chan func(*earThread))
		go e.loop()
	})
	done := make(chan struct{})
	e.reqs <- func(t *earThread) { f(t); close(done) }
	<-done
}

func (e *winEar) Available() bool {
	var ok bool
	e.do(func(t *earThread) { ok = t.available() })
	return ok
}

func (e *winEar) Start(locale string) error {
	var err error
	e.do(func(t *earThread) {
		if t.session != nil {
			err = speechFailure{Code: 4}
			return
		}
		sep := ""
		if strings.HasPrefix(locale, "en") {
			sep = " "
		}
		e.mu.Lock()
		e.final, e.hyp, e.sep, e.errCode = "", "", sep, 0
		e.mu.Unlock()
		t.session, err = begin(e, locale)
		if err == nil {
			e.mu.Lock()
			e.listening = true
			e.mu.Unlock()
		}
	})
	return err
}

func (e *winEar) Stop() (string, error) {
	e.do(func(t *earThread) {
		if t.session == nil {
			return
		}
		t.session.close()
		t.session = nil
		e.mu.Lock()
		e.listening = false
		e.mu.Unlock()
	})
	return e.View().Text, nil
}

func (e *winEar) View() SpeechView {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SpeechView{Listening: e.listening, Text: e.text(), ErrCode: e.errCode}
}

func (e *winEar) text() string {
	if e.hyp == "" {
		return e.final
	}
	if e.final == "" {
		return e.hyp
	}
	return e.final + e.sep + e.hyp
}

// heard takes a phrase from the recognizer: a hypothesis replaces the one
// before it, a recognition is kept and the next phrase starts after it.
func (e *winEar) heard(text string, done bool) {
	text = strings.TrimSpace(text)
	e.mu.Lock()
	defer e.mu.Unlock()
	if !done {
		e.hyp = text
		return
	}
	e.hyp = ""
	if text == "" {
		return
	}
	if e.final != "" {
		e.final += e.sep
	}
	e.final += text
}

// earThread is the state only the COM thread touches.
type earThread struct {
	session   *recoSession
	availAt   time.Time
	availWas  bool
	availSeen bool
}

func (t *earThread) available() bool {
	if t.availSeen && (t.availWas || time.Since(t.availAt) < availableFor) {
		return t.availWas
	}
	t.availSeen, t.availAt, t.availWas = true, time.Now(), probeRecognizer()
	return t.availWas
}

func (e *winEar) loop() {
	// never unlocked: this thread is the apartment for the app's lifetime
	runtime.LockOSThread()
	// S_FALSE (already initialized here) is fine; if COM is missing
	// altogether, creating the recognizer fails and Available says so
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	t := &earThread{}
	for {
		select {
		case f := <-e.reqs:
			f(t)
		default:
			if t.session == nil {
				// nothing to pump: wait for the next request
				f := <-e.reqs
				f(t)
				continue
			}
			if !pumpMessages() {
				time.Sleep(20 * time.Millisecond)
			}
		}
	}
}

// probeRecognizer says whether SAPI has an in-process recognizer for one of
// the window's languages.
func probeRecognizer() bool {
	reco, err := newRecognizer()
	if err != nil {
		return false
	}
	defer reco.Release()
	for _, l := range []string{"zh-CN", "en-US"} {
		lang, _ := speechLangHex(l)
		if tok, err := recognizerFor(reco, lang); err == nil {
			tok.Release()
			return true
		}
	}
	return false
}

func newRecognizer() (*ole.IDispatch, error) {
	unk, err := oleutil.CreateObject("SAPI.SpInprocRecognizer")
	if err != nil {
		return nil, err
	}
	defer unk.Release()
	return unk.QueryInterface(ole.IID_IDispatch)
}

func begin(e *winEar, locale string) (*recoSession, error) {
	lang, ok := speechLangHex(locale)
	if !ok {
		return nil, speechFailure{Code: 3}
	}
	reco, err := newRecognizer()
	if err != nil {
		return nil, speechFailure{Code: speechHRESULT(err)}
	}
	s := &recoSession{reco: reco}
	fail := func(code int) (*recoSession, error) {
		s.close()
		return nil, speechFailure{Code: code}
	}
	token, err := recognizerFor(reco, lang)
	if err != nil {
		return fail(3)
	}
	// Recognizer and AudioInput are object properties: set by reference
	_, err = oleutil.PutPropertyRef(reco, "Recognizer", token)
	token.Release()
	if err != nil {
		return fail(speechHRESULT(err))
	}
	// an in-process recognizer has no microphone until it is given one
	mic, err := firstToken(reco, "GetAudioInputs")
	if err != nil {
		return fail(2)
	}
	_, err = oleutil.PutPropertyRef(reco, "AudioInput", mic)
	mic.Release()
	if err != nil {
		return fail(2)
	}
	ctxv, err := oleutil.CallMethod(reco, "CreateRecoContext")
	if err != nil {
		return fail(speechHRESULT(err))
	}
	s.ctx = ctxv.ToIDispatch()
	s.sink = newSpeechSink(e)
	if s.point, s.cookie, err = advise(s.ctx, s.sink); err != nil {
		return fail(3)
	}
	const hypothesis, recognition = 32, 16
	if _, err := oleutil.PutProperty(s.ctx, "EventInterests", int32(hypothesis|recognition)); err != nil {
		return fail(3)
	}
	gv, err := oleutil.CallMethod(s.ctx, "CreateGrammar", 0)
	if err != nil {
		return fail(speechHRESULT(err))
	}
	s.grammar = gv.ToIDispatch()
	if _, err := oleutil.CallMethod(s.grammar, "DictationLoad", "", 0); err != nil {
		return fail(3)
	}
	if _, err := oleutil.CallMethod(s.grammar, "DictationSetState", 1); err != nil {
		return fail(speechHRESULT(err))
	}
	return s, nil
}

// recognizerFor is the first recognizer token for a language, given as
// hex (804) or, on some systems, decimal (2052).
func recognizerFor(reco *ole.IDispatch, lang string) (*ole.IDispatch, error) {
	for _, attr := range []string{"Language=" + lang, "Language=" + langDecimal(lang)} {
		if tok, err := firstToken(reco, "GetRecognizers", attr, ""); err == nil {
			return tok, nil
		}
	}
	return nil, speechFailure{Code: 3}
}

// firstToken calls a SAPI method that returns a token collection and gives
// back its first item.
func firstToken(reco *ole.IDispatch, method string, args ...any) (*ole.IDispatch, error) {
	v, err := oleutil.CallMethod(reco, method, args...)
	if err != nil {
		return nil, err
	}
	if v.VT != ole.VT_DISPATCH {
		v.Clear()
		return nil, speechFailure{Code: 3}
	}
	tokens := v.ToIDispatch()
	defer tokens.Release()
	cv, err := oleutil.GetProperty(tokens, "Count")
	if err != nil {
		return nil, err
	}
	n := cv.Val
	cv.Clear()
	if n < 1 {
		return nil, speechFailure{Code: 3}
	}
	item, err := oleutil.CallMethod(tokens, "Item", 0)
	if err != nil {
		return nil, err
	}
	if item.VT != ole.VT_DISPATCH {
		item.Clear()
		return nil, speechFailure{Code: 3}
	}
	return item.ToIDispatch(), nil
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
	if oe, ok := err.(*ole.OleError); ok && uint32(oe.Code()) == 0x80070005 {
		return 2 // E_ACCESSDENIED: the microphone is turned off for apps
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "80070005") || strings.Contains(s, "access is denied") {
		return 2
	}
	return 3
}

type recoSession struct {
	reco    *ole.IDispatch
	ctx     *ole.IDispatch
	grammar *ole.IDispatch
	// sink is only referenced by SAPI otherwise: the session keeps it alive
	// for as long as SAPI may call it
	sink   *speechSink
	point  *ole.IConnectionPoint
	cookie uint32
}

// close stops listening, takes the last phrase that is still on its way,
// and releases everything.
func (s *recoSession) close() {
	if s.grammar != nil {
		oleutil.CallMethod(s.grammar, "DictationSetState", 0)
		for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); {
			if !pumpMessages() {
				time.Sleep(20 * time.Millisecond)
			}
		}
		s.grammar.Release()
	}
	if s.point != nil {
		s.point.Unadvise(s.cookie)
		s.point.Release()
	}
	if s.ctx != nil {
		s.ctx.Release()
	}
	if s.reco != nil {
		s.reco.Release()
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

// speechSink is a minimal IDispatch that SAPI calls with its events.
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

func sinkQuery(this *speechSink, iid *ole.GUID, punk *unsafe.Pointer) uintptr {
	*punk = nil
	if ole.IsEqualGUID(iid, ole.IID_IUnknown) || ole.IsEqualGUID(iid, ole.IID_IDispatch) || ole.IsEqualGUID(iid, iidRecoEvents) {
		atomic.AddInt32(&this.ref, 1)
		*punk = unsafe.Pointer(this)
		return ole.S_OK
	}
	return ole.E_NOINTERFACE
}

func sinkAdd(this *speechSink) uintptr { return uintptr(atomic.AddInt32(&this.ref, 1)) }

func sinkRelease(this *speechSink) uintptr { return uintptr(atomic.AddInt32(&this.ref, -1)) }

// GetTypeInfoCount(this, *count): no type information.
func sinkCount(this *speechSink, n *uint32) uintptr {
	if n != nil {
		*n = 0
	}
	return ole.S_OK
}

// GetTypeInfo(this, index, lcid, **info)
func sinkInfo(this *speechSink, index, lcid uintptr, info *unsafe.Pointer) uintptr {
	if info != nil {
		*info = nil
	}
	return ole.E_NOTIMPL
}

// GetIDsOfNames(this, riid, names, count, lcid, ids): SAPI calls by dispid.
func sinkNames(this *speechSink, riid, names, count, lcid, ids uintptr) uintptr {
	return ole.E_NOTIMPL
}

// Invoke(this, dispid, riid, lcid, flags, params, result, excep, argErr).
// Recognition is dispid 7 and Hypothesis 8; the result object is the last
// argument, which DISPPARAMS lists first.
func sinkInvoke(this *speechSink, dispid, riid, lcid, flags uintptr, params *dispParams, result, excep, argErr uintptr) uintptr {
	if (dispid != 7 && dispid != 8) || this.ear == nil {
		return ole.S_OK
	}
	if text, ok := phraseText(params); ok {
		this.ear.heard(text, dispid == 7)
	}
	return ole.S_OK
}

// dispParams is ole.DISPPARAMS with its fields visible.
type dispParams struct {
	rgvarg *ole.VARIANT
	named  uintptr
	cArgs  uint32
	cNamed uint32
}

func phraseText(params *dispParams) (string, bool) {
	if params == nil || params.cArgs == 0 || params.rgvarg == nil {
		return "", false
	}
	v := params.rgvarg
	if v.VT != ole.VT_DISPATCH {
		return "", false
	}
	result := v.ToIDispatch()
	if result == nil {
		return "", false
	}
	pv, err := oleutil.GetProperty(result, "PhraseInfo")
	if err != nil {
		return "", false
	}
	defer pv.Clear()
	if pv.VT != ole.VT_DISPATCH {
		return "", false
	}
	tv, err := oleutil.CallMethod(pv.ToIDispatch(), "GetText")
	if err != nil {
		return "", false
	}
	defer tv.Clear()
	return tv.ToString(), true
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

// pumpMessages dispatches what is waiting for this thread (SAPI posts its
// events here) and says whether there was anything.
func pumpMessages() bool {
	got := false
	for {
		var m winMSG
		const pmRemove = 1
		if r, _, _ := peekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove); r == 0 {
			return got
		}
		got = true
		translate.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}
