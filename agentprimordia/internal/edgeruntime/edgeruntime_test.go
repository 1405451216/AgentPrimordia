package edgeruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// FromHTTPRequest / ToHTTPRequest / WriteHTTPResponse / FromHTTPResponse
// ===========================================================================

func TestFromHTTPRequest_Basic(t *testing.T) {
	body := strings.NewReader(`{"x":1}`)
	r := httptest.NewRequest("POST", "http://example.com/foo?q=1", body)
	r.Header.Set("X-Trace-Id", "abc")

	e, err := FromHTTPRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if e.Method != "POST" {
		t.Fatalf("Method=%s", e.Method)
	}
	if !strings.Contains(e.URL, "/foo") {
		t.Fatalf("URL=%s", e.URL)
	}
	if string(e.Body) != `{"x":1}` {
		t.Fatalf("Body=%q", string(e.Body))
	}
	if e.HeaderValue("X-Trace-Id") != "abc" {
		t.Fatalf("X-Trace-Id=%q", e.HeaderValue("X-Trace-Id"))
	}
}

func TestFromHTTPRequest_Nil(t *testing.T) {
	if _, err := FromHTTPRequest(nil); err == nil {
		t.Fatal("nil 应报错")
	}
}

func TestToHTTPRequest_RoundTrip(t *testing.T) {
	e := &EdgeRequest{
		Method:  "GET",
		URL:     "http://example.com/x",
		Headers: map[string]string{"X-Test": "1"},
	}
	httpReq, err := ToHTTPRequest(e)
	if err != nil {
		t.Fatal(err)
	}
	if httpReq.Method != "GET" || httpReq.Header.Get("X-Test") != "1" {
		t.Fatalf("转换异常：%+v", httpReq)
	}
}

func TestToHTTPRequest_NoURL(t *testing.T) {
	if _, err := ToHTTPRequest(&EdgeRequest{Method: "GET"}); err == nil {
		t.Fatal("缺 URL 应报错")
	}
}

func TestToHTTPRequest_Nil(t *testing.T) {
	if _, err := ToHTTPRequest(nil); err == nil {
		t.Fatal("nil 应报错")
	}
}

func TestWriteHTTPResponse(t *testing.T) {
	e := &EdgeResponse{
		Status:  http.StatusCreated,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"ok":true}`),
	}
	rr := httptest.NewRecorder()
	if err := WriteHTTPResponse(rr, e); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusCreated {
		t.Fatalf("Code=%d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("CT=%q", rr.Header().Get("Content-Type"))
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("Body=%q", rr.Body.String())
	}
}

func TestWriteHTTPResponse_DefaultsTo200(t *testing.T) {
	rr := httptest.NewRecorder()
	if err := WriteHTTPResponse(rr, &EdgeResponse{}); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("Code=%d", rr.Code)
	}
}

func TestWriteHTTPResponse_Nil(t *testing.T) {
	rr := httptest.NewRecorder()
	if err := WriteHTTPResponse(rr, nil); err == nil {
		t.Fatal("nil 应报错")
	}
}

func TestFromHTTPResponse_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "1")
		w.WriteHeader(200)
		w.Write([]byte("hi"))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	e, err := FromHTTPResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != 200 || string(e.Body) != "hi" || e.HeaderValue("X-Test") != "1" {
		t.Fatalf("EdgeResponse=%+v", e)
	}
}

// ===========================================================================
// HTTPFetcher
// ===========================================================================

func TestHTTPFetcher_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.WriteHeader(200)
		w.Write([]byte("echo:" + string(b)))
	}))
	defer srv.Close()

	f := NewHTTPFetcher()
	resp, err := f.Fetch(context.Background(), &EdgeRequest{
		Method:  "POST",
		URL:     srv.URL,
		Headers: map[string]string{"Content-Type": "text/plain"},
		Body:    []byte("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || string(resp.Body) != "echo:hello" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestHTTPFetcher_DefaultClient(t *testing.T) {
	f := NewHTTPFetcher()
	if f.Client == nil {
		t.Fatal("default client 应非 nil")
	}
}

// ===========================================================================
// Headers 工具
// ===========================================================================

func TestMergeHeaders(t *testing.T) {
	a := map[string]string{"X-A": "1"}
	b := map[string]string{"X-B": "2"}
	c := map[string]string{"X-A": "3"}

	merged := MergeHeaders(a, b, c)
	if merged["X-A"] != "3" || merged["X-B"] != "2" {
		t.Fatalf("merged=%v", merged)
	}
	if len(merged) != 2 {
		t.Fatalf("len=%d", len(merged))
	}
}

func TestEdgeRequest_ContentType(t *testing.T) {
	r := &EdgeRequest{Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"}}
	if r.ContentType() != "application/json" {
		t.Fatalf("CT=%q", r.ContentType())
	}

	r2 := &EdgeRequest{}
	if r2.ContentType() != "application/octet-stream" {
		t.Fatalf("默认 CT=%q", r2.ContentType())
	}
}

func TestEdgeRequest_HeaderValue_Nil(t *testing.T) {
	r := &EdgeRequest{}
	if r.HeaderValue("X") != "" {
		t.Fatal("nil Headers 应返回空")
	}
}

func TestEdgeResponse_HeaderValue_Nil(t *testing.T) {
	r := &EdgeResponse{}
	if r.HeaderValue("X") != "" {
		t.Fatal("nil Headers 应返回空")
	}
}

// ===========================================================================
// WithTimeout
// ===========================================================================

func TestWithTimeout(t *testing.T) {
	ctx, cancel := WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		// 预期
	case <-time.After(100 * time.Millisecond):
		t.Fatal("应超时")
	}
}

func TestWithTimeout_NilParent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil parent 不应 panic：%v", r)
		}
	}()
	ctx, cancel := WithTimeout(nil, time.Millisecond)
	defer cancel()
	if ctx == nil {
		t.Fatal("ctx 不应为 nil")
	}
}

// ===========================================================================
// MemoryKV
// ===========================================================================

func TestMemoryKV_PutGet(t *testing.T) {
	kv := NewMemoryKV()
	if err := kv.Put("k1", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	got, err := kv.Get("k1")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1" {
		t.Fatalf("got=%q", string(got))
	}
}

func TestMemoryKV_GetMissing(t *testing.T) {
	kv := NewMemoryKV()
	if _, err := kv.Get("missing"); !errors.Is(err, ErrKVKeyNotFound) {
		t.Fatalf("missing 应返回 ErrKVKeyNotFound，实际=%v", err)
	}
}

func TestMemoryKV_PutOverwrite(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("k", []byte("v1"))
	_ = kv.Put("k", []byte("v2"))

	got, _ := kv.Get("k")
	if string(got) != "v2" {
		t.Fatal("Put 应覆盖")
	}
}

func TestMemoryKV_Delete(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("k", []byte("v"))
	if err := kv.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get("k"); !errors.Is(err, ErrKVKeyNotFound) {
		t.Fatal("删除后应找不到")
	}
}

func TestMemoryKV_DeleteMissing(t *testing.T) {
	kv := NewMemoryKV()
	if err := kv.Delete("missing"); !errors.Is(err, ErrKVKeyNotFound) {
		t.Fatalf("missing 应返回 ErrKVKeyNotFound，实际=%v", err)
	}
}

func TestMemoryKV_PutEmptyKey(t *testing.T) {
	kv := NewMemoryKV()
	if err := kv.Put("", []byte("v")); err == nil {
		t.Fatal("空 key 应报错")
	}
}

func TestMemoryKV_GetEmptyKey(t *testing.T) {
	kv := NewMemoryKV()
	if _, err := kv.Get(""); err == nil {
		t.Fatal("空 key 应报错")
	}
}

func TestMemoryKV_TTL(t *testing.T) {
	kv := NewMemoryKV()
	if err := kv.Put("k", []byte("v"), WithExpirationTTL(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	// 立即获取应成功
	if _, err := kv.Get("k"); err != nil {
		t.Fatal("立即获取应成功")
	}
	// 等待过期
	time.Sleep(100 * time.Millisecond)
	if _, err := kv.Get("k"); !errors.Is(err, ErrKVKeyNotFound) {
		t.Fatal("过期后应找不到")
	}
}

func TestMemoryKV_AbsoluteExpiration(t *testing.T) {
	kv := NewMemoryKV()
	if err := kv.Put("k", []byte("v"), WithExpiration(time.Now().Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get("k"); !errors.Is(err, ErrKVKeyNotFound) {
		t.Fatal("绝对过期时间已过去，应返回 NotFound")
	}
}

func TestMemoryKV_Metadata(t *testing.T) {
	kv := NewMemoryKV()
	if err := kv.Put("k", []byte("v"), WithMetadata(map[string]string{"a": "1"})); err != nil {
		t.Fatal(err)
	}
	e, err := kv.GetWithMetadata("k")
	if err != nil {
		t.Fatal(err)
	}
	if e.Metadata["a"] != "1" {
		t.Fatalf("metadata=%v", e.Metadata)
	}
}

func TestMemoryKV_List_Prefix(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("user:1", []byte("a"))
	_ = kv.Put("user:2", []byte("b"))
	_ = kv.Put("post:1", []byte("c"))

	list, err := kv.List(&KVListOptions{Prefix: "user:"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	for _, e := range list {
		if !strings.HasPrefix(e.Key, "user:") {
			t.Fatalf("前缀过滤错误：%s", e.Key)
		}
	}
}

func TestMemoryKV_List_Limit(t *testing.T) {
	kv := NewMemoryKV()
	for i := 0; i < 10; i++ {
		_ = kv.Put("k"+string(rune('0'+i)), []byte("v"))
	}
	list, _ := kv.List(&KVListOptions{Limit: 3})
	if len(list) != 3 {
		t.Fatalf("len=%d", len(list))
	}
}

func TestMemoryKV_List_Reverse(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("a", []byte("1"))
	_ = kv.Put("b", []byte("2"))
	_ = kv.Put("c", []byte("3"))

	list, _ := kv.List(&KVListOptions{Reverse: true})
	if list[0].Key != "c" || list[2].Key != "a" {
		t.Fatalf("reverse=%v", list)
	}
}

func TestMemoryKV_List_PurgeExpired(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("a", []byte("1"))
	_ = kv.Put("b", []byte("2"), WithExpiration(time.Now().Add(-time.Second)))

	list, _ := kv.List(nil)
	if len(list) != 1 {
		t.Fatalf("过期应在 List 时清理：len=%d", len(list))
	}
}

func TestMemoryKV_PurgeExpired(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("a", []byte("1"))
	_ = kv.Put("b", []byte("2"), WithExpiration(time.Now().Add(-time.Second)))

	n := kv.PurgeExpired()
	if n != 1 {
		t.Fatalf("PurgeExpired=%d", n)
	}
	if kv.Len() != 1 {
		t.Fatalf("Len=%d", kv.Len())
	}
}

func TestMemoryKV_Len(t *testing.T) {
	kv := NewMemoryKV()
	if kv.Len() != 0 {
		t.Fatal("空 KV Len 应=0")
	}
	_ = kv.Put("a", []byte("1"))
	_ = kv.Put("b", []byte("2"))
	if kv.Len() != 2 {
		t.Fatalf("Len=%d", kv.Len())
	}
}

func TestMemoryKV_GetReturnsCopy(t *testing.T) {
	kv := NewMemoryKV()
	_ = kv.Put("k", []byte("hello"))

	v1, _ := kv.Get("k")
	v1[0] = 'X'

	v2, _ := kv.Get("k")
	if v2[0] == 'X' {
		t.Fatal("返回的应是副本，不应被修改影响")
	}
}

// ===========================================================================
// SystemClock / FakeClock
// ===========================================================================

func TestSystemClock_Now(t *testing.T) {
	before := time.Now()
	c := NewSystemClock()
	got := c.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Fatalf("SystemClock.Now() 越界：%v 在 %v-%v 之间", got, before, after)
	}
}

func TestSystemClock_NewTimer(t *testing.T) {
	c := NewSystemClock()
	tm := c.NewTimer(10 * time.Millisecond)
	defer tm.Stop()

	select {
	case <-tm.C():
		// 预期
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timer 应触发")
	}
}

func TestFakeClock_NowAdvance(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := NewFakeClock(now)
	if !fc.Now().Equal(now) {
		t.Fatal("Now 起始不匹配")
	}
	fc.Advance(time.Hour)
	expected := now.Add(time.Hour)
	if !fc.Now().Equal(expected) {
		t.Fatalf("Advance 后=%v，期望=%v", fc.Now(), expected)
	}
}

func TestFakeClock_SetNow(t *testing.T) {
	fc := NewFakeClockNow()
	target := time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC)
	fc.SetNow(target)
	if !fc.Now().Equal(target) {
		t.Fatal("SetNow 未生效")
	}
}

func TestFakeClock_TimerFiresOnAdvance(t *testing.T) {
	fc := NewFakeClock(time.Now())
	tm := fc.NewTimer(time.Hour)

	fc.Advance(time.Hour)

	select {
	case <-tm.C():
		// 预期
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Advance 后 timer 应触发")
	}
}

func TestFakeClock_TimerNotFiredBeforeAdvance(t *testing.T) {
	fc := NewFakeClock(time.Now())
	tm := fc.NewTimer(time.Hour)
	defer tm.Stop()

	select {
	case <-tm.C():
		t.Fatal("未 Advance 不应触发")
	case <-time.After(20 * time.Millisecond):
		// 预期
	}
}

func TestFakeClock_StopTimer(t *testing.T) {
	fc := NewFakeClock(time.Now())
	tm := fc.NewTimer(time.Hour)
	if !tm.Stop() {
		t.Fatal("首次 Stop 应返回 true")
	}
	if tm.Stop() {
		t.Fatal("二次 Stop 应返回 false")
	}
}

// ===========================================================================
// 并发安全：MemoryKV 并发读写
// ===========================================================================

func TestMemoryKV_Concurrent(t *testing.T) {
	kv := NewMemoryKV()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = kv.Put("k", []byte("v"))
			time.Sleep(time.Microsecond)
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		_, _ = kv.Get("k")
		_, _ = kv.List(nil)
	}
	<-done
}

// ===========================================================================
// EdgeRequest Body 比较
// ===========================================================================

func TestEdgeRequest_BodyBytes(t *testing.T) {
	e := &EdgeRequest{Body: []byte("hello")}
	if !bytes.Equal(e.Body, []byte("hello")) {
		t.Fatal("Body 字段损坏")
	}
}