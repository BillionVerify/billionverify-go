package billionverify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type recorded struct {
	method string
	path   string
	query  string
	header http.Header
	body   []byte
}

// newTestClient starts a server answering with handler and returns a client for it
// whose waits are recorded instead of slept.
func newTestClient(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, rec recorded)) (*Client, *[]recorded, *[]time.Duration) {
	t.Helper()
	var reqs []recorded
	var sleeps []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec := recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), b}
		reqs = append(reqs, rec)
		handler(w, r, rec)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(Config{APIKey: "test-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	c.sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	return c, &reqs, &sleeps
}

func ok(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"code":"0","message":"Success","data":` + data + `}`))
}

func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"success":false,"code":"` + strconv.Itoa(status) + `","message":"` + message + `","error":{"code":"` + code + `","message":"` + message + `"}}`))
}

func bodyMap(t *testing.T, rec recorded) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(rec.body, &m); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.body)
	}
	return m
}

func multipartFields(t *testing.T, rec recorded) (fields map[string]string, filename, content string) {
	t.Helper()
	_, params, err := mime.ParseMediaType(rec.header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content type: %v", err)
	}
	r := multipart.NewReader(strings.NewReader(string(rec.body)), params["boundary"])
	fields = map[string]string{}
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		if p.FormName() == "file" {
			filename, content = p.FileName(), string(b)
		} else {
			fields[p.FormName()] = string(b)
		}
	}
	return
}

func TestAuthHeaderIsBVAPIKeyEverywhere(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { ok(w, `{}`) })
	ctx := context.Background()
	_, _ = c.GetCredits(ctx)
	_, _ = c.Verify(ctx, "a@b.co", nil)
	_, _ = c.UploadFileReader(ctx, strings.NewReader("a@b.co\n"), "x.csv", nil)
	_, _ = c.GetFileJobResults(ctx, "j", nil)
	_, _ = c.CheckDisposable(ctx, "a@b.co")
	for _, r := range *reqs {
		if r.header.Get("BV-API-KEY") != "test-key" {
			t.Errorf("%s %s: missing BV-API-KEY", r.method, r.path)
		}
		if r.header.Get("EV-API-KEY") != "" {
			t.Errorf("%s %s: sent legacy EV-API-KEY", r.method, r.path)
		}
	}
}

func TestBaseURLWithV1IsAccepted(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k", BaseURL: "https://api.billionverify.com/v1/"})
	if err != nil || c.baseURL != "https://api.billionverify.com" {
		t.Fatalf("got %v %v", c, err)
	}
}

func TestVerifyOptions(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"email":"a@b.co","status":"valid","check_smtp":true,"verification_mode":"smtp","retryable":false,"risk_reasons":["x"],"retry_after_seconds":30}`)
	})
	ctx := context.Background()

	res, err := c.Verify(ctx, "a@b.co", &VerifyOptions{ServerDefaultSMTP: true, ForceRefresh: true, IncludeDomainReputation: true})
	if err != nil {
		t.Fatal(err)
	}
	m := bodyMap(t, (*reqs)[0])
	if _, has := m["check_smtp"]; has {
		t.Error("check_smtp must be left out with ServerDefaultSMTP")
	}
	if m["force_refresh"] != true || m["include_domain_reputation"] != true {
		t.Errorf("options missing: %v", m)
	}
	if !res.CheckedSMTP || !res.SMTPCheck || res.VerificationMode != "smtp" || len(res.RiskReasons) != 1 || *res.RetryAfterSeconds != 30 {
		t.Errorf("response fields: %+v", res)
	}

	// legacy behaviour is kept: no options sends check_smtp=true, options send CheckSMTP
	_, _ = c.Verify(ctx, "a@b.co", nil)
	if bodyMap(t, (*reqs)[1])["check_smtp"] != true {
		t.Error("nil options should send check_smtp=true")
	}
	_, _ = c.Verify(ctx, "a@b.co", &VerifyOptions{CheckSMTP: false})
	if bodyMap(t, (*reqs)[2])["check_smtp"] != false {
		t.Error("CheckSMTP=false should be sent")
	}
}

func TestVerifyBatchResponseAndOptions(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"results":[{"email":"a@b.co","status":"valid","credits_used":1,"verification_mode":"smtp"}],"total_emails":1,
			"retryable_emails":["c@d.co"],"deferred_emails":["e@f.co"],"retry_after_seconds":60,
			"deliverability_buckets":{"strict_deliverable":1,"not_deliverable":2,"transient_failure":3}}`)
	})
	res, err := c.VerifyBatch(context.Background(), []string{"a@b.co"}, &BatchVerifyOptions{ServerDefaultSMTP: true, ForceRefresh: true, IncludeDomainReputation: true})
	if err != nil {
		t.Fatal(err)
	}
	m := bodyMap(t, (*reqs)[0])
	if _, has := m["check_smtp"]; has || m["force_refresh"] != true || m["include_domain_reputation"] != true {
		t.Errorf("body: %v", m)
	}
	if res.DeliverabilityBuckets == nil || res.DeliverabilityBuckets.NotDeliverable != 2 || res.DeliverabilityBuckets.TransientFailure != 3 {
		t.Errorf("buckets: %+v", res.DeliverabilityBuckets)
	}
	if len(res.RetryableEmails) != 1 || len(res.DeferredEmails) != 1 || *res.RetryAfterSeconds != 60 {
		t.Errorf("retry lists: %+v", res)
	}
	// a row that leaves optional fields out still parses
	if res.Results[0].Domain != "" || res.Results[0].Reason != "" || res.Results[0].VerificationMode != "smtp" {
		t.Errorf("row: %+v", res.Results[0])
	}
}

func TestVerifyBatchRefusesMoreThanFifty(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { ok(w, `{}`) })
	_, err := c.VerifyBatch(context.Background(), make([]string, 51), nil)
	var ve *ValidationError
	if !errors.As(err, &ve) || len(*reqs) != 0 {
		t.Fatalf("err=%v requests=%d", err, len(*reqs))
	}
}

func TestCheckDisposable(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"email":"x@mailinator.com","domain":"mailinator.com","is_disposable":true,"checked_at":"2026-01-01T00:00:00Z"}`)
	})
	ctx := context.Background()
	a, err := c.CheckDisposable(ctx, "x@mailinator.com")
	if err != nil || !a.IsDisposable || a.Domain != "mailinator.com" {
		t.Fatalf("%+v %v", a, err)
	}
	b, err := c.CheckDisposableViaQuery(ctx, "x+t@mailinator.com")
	if err != nil || !b.IsDisposable {
		t.Fatalf("%+v %v", b, err)
	}
	if (*reqs)[0].method != "POST" || (*reqs)[0].path != "/v1/verify/disposable" || bodyMap(t, (*reqs)[0])["email"] != "x@mailinator.com" {
		t.Errorf("post: %+v", (*reqs)[0])
	}
	if (*reqs)[1].method != "GET" || (*reqs)[1].query != "email=x%2Bt%40mailinator.com" {
		t.Errorf("get: %+v", (*reqs)[1])
	}
}

func TestFileJobStatusModel(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"task_id":"job-1","status":"completed_with_warning","stage":"generating_report","progress":100,"email_progress":100,
			"total_emails":10,"risky_emails":2,"credits_used":7,"can_resume":false,"direct_download_url":"https://s.example/x.csv",
			"result_download_available":true,"runtime_hints":{"inflight_batches":4},"capacity_wait":{"waiting_emails":3},
			"deliverability_buckets":{"unknown":2},"unique_emails":9,"total_rows":10}`)
	})
	st, err := c.GetFileJobStatus(context.Background(), "job-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.TaskID != "job-1" || st.JobID != "job-1" || st.Progress != 100 || st.ProgressPercent != 100 {
		t.Errorf("ids/progress: %+v", st)
	}
	if st.Status != JobStatusCompletedWithWarning || st.Stage != "generating_report" || st.RiskyEmails != 2 {
		t.Errorf("status: %+v", st)
	}
	if st.DeliverabilityBuckets.Unknown != 2 || *st.RuntimeHints.InflightBatches != 4 || *st.CapacityWait.WaitingEmails != 3 || *st.UniqueEmails != 9 || *st.CanResume {
		t.Errorf("nested: %+v", st)
	}
	if !st.ResultDownloadAvailable || st.DirectDownloadURL == "" {
		t.Errorf("download: %+v", st)
	}
}

func TestFileJobStatusLongPollingAndLimits(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"task_id":"j","status":"running"}`)
	})
	ctx := context.Background()
	if _, err := c.GetFileJobStatus(ctx, "a/b c", &FileJobStatusOptions{Timeout: 60}); err != nil {
		t.Fatal(err)
	}
	if (*reqs)[0].path != "/v1/verify/file/a/b c" && (*reqs)[0].path != "/v1/verify/file/a%2Fb%20c" {
		t.Errorf("path: %q", (*reqs)[0].path)
	}
	if (*reqs)[0].query != "timeout=60" {
		t.Errorf("query: %q", (*reqs)[0].query)
	}
	if _, err := c.GetFileJobStatus(ctx, "j", &FileJobStatusOptions{Timeout: -1}); err == nil {
		t.Error("negative timeout must be rejected")
	}
	if _, err := c.GetFileJobStatus(ctx, "j", &FileJobStatusOptions{Timeout: 999}); err != nil || (*reqs)[1].query != "timeout=300" {
		t.Errorf("timeout above 300 is clamped: %v %q", err, (*reqs)[1].query)
	}
}

func TestLongPollIsNotCutOffByTheClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		ok(w, `{"task_id":"j","status":"completed"}`)
	}))
	defer srv.Close()
	c, _ := NewClient(Config{APIKey: "k", BaseURL: srv.URL, Timeout: 100 * time.Millisecond})
	if _, err := c.GetFileJobStatus(context.Background(), "j", nil); err == nil {
		t.Fatal("expected the plain call to time out")
	} else {
		var te *TimeoutError
		if !errors.As(err, &te) {
			t.Fatalf("want TimeoutError, got %T %v", err, err)
		}
	}
	if st, err := c.GetFileJobStatus(context.Background(), "j", &FileJobStatusOptions{Timeout: 5}); err != nil || st.Status != JobStatusCompleted {
		t.Fatalf("long poll: %v %v", st, err)
	}
}

func TestLegacyJobFieldNamesStillDecode(t *testing.T) {
	var r FileJobResponse
	if err := json.Unmarshal([]byte(`{"job_id":"old","progress_percent":40,"status":"processing"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.TaskID != "old" || r.JobID != "old" || r.Progress != 40 || r.ProgressPercent != 40 {
		t.Errorf("%+v", r)
	}
}

func TestWaitForFileJobCompletion(t *testing.T) {
	var n int32
	statuses := []string{"running", "merging", "completed_with_warning"}
	c, _, sleeps := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		i := int(atomic.AddInt32(&n, 1)) - 1
		ok(w, `{"task_id":"j","status":"`+statuses[i]+`"}`)
	})
	st, err := c.WaitForFileJobCompletion(context.Background(), "j", 7*time.Second, time.Minute)
	if err != nil || st.Status != JobStatusCompletedWithWarning {
		t.Fatalf("%v %v", st, err)
	}
	if len(*sleeps) != 2 || (*sleeps)[0] != 7*time.Second {
		t.Errorf("sleeps: %v", *sleeps)
	}
	for _, s := range []JobStatus{JobStatusCompleted, JobStatusCompletedWithWarning, JobStatusFailed, JobStatusCancelled, JobStatusPaused} {
		if !s.IsSettled() {
			t.Errorf("%s should settle", s)
		}
	}
	for _, s := range []JobStatus{JobStatusPreparing, JobStatusPending, JobStatusRunning, JobStatusProcessing, JobStatusMerging} {
		if s.IsSettled() {
			t.Errorf("%s should not settle", s)
		}
	}
}

func TestWaitForFileJobCompletionStopsOnPausedAndTimesOut(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { ok(w, `{"status":"paused"}`) })
	if st, err := c.WaitForFileJobCompletion(context.Background(), "j", time.Second, time.Minute); err != nil || st.Status != JobStatusPaused {
		t.Fatalf("%v %v", st, err)
	}
	c2, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { ok(w, `{"status":"running"}`) })
	_, err := c2.WaitForFileJobCompletion(context.Background(), "j", time.Millisecond, time.Millisecond)
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("want TimeoutError, got %v", err)
	}
}

func TestGetFileJobResultsFilters(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { _, _ = w.Write([]byte("email,status\n")) })
	yes, no := true, false
	csv, err := c.GetFileJobResults(context.Background(), "job-1", &FileResultsOptions{Valid: &yes, Risky: &yes, Invalid: &no})
	if err != nil || string(csv) != "email,status\n" {
		t.Fatalf("%q %v", csv, err)
	}
	if q := (*reqs)[0].query; q != "invalid=false&risky=true&valid=true" {
		t.Errorf("query: %s", q)
	}
}

func TestResultsRedirectDoesNotLeakTheKey(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("BV-API-KEY") != "" || r.Header.Get("EV-API-KEY") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("credentials reached the storage host: %v", r.Header)
		}
		_, _ = w.Write([]byte("csv-data"))
	}))
	defer storage.Close()
	// a different port is a different origin
	target := storage.URL
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		http.Redirect(w, r, target+"/r/job-1.csv?sig=abc", http.StatusTemporaryRedirect)
	})
	got, err := c.GetFileJobResults(context.Background(), "job-1", nil)
	if err != nil || string(got) != "csv-data" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestResultsRedirectOnTheSameHostKeepsTheKey(t *testing.T) {
	var hops int32
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		if atomic.AddInt32(&hops, 1) == 1 {
			http.Redirect(w, r, "/v1/files/x", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	got, err := c.GetFileJobResults(context.Background(), "job-1", nil)
	if err != nil || string(got) != "ok" || (*reqs)[1].header.Get("BV-API-KEY") != "test-key" || (*reqs)[1].path != "/v1/files/x" {
		t.Fatalf("%q %v %+v", got, err, *reqs)
	}
}

func TestRedirectLoopGivesUp(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		http.Redirect(w, r, r.URL.Path, http.StatusTemporaryRedirect)
	})
	_, err := c.GetFileJobResults(context.Background(), "job-1", nil)
	var be *BillionVerifyError
	if !errors.As(err, &be) || be.Code != "TOO_MANY_REDIRECTS" {
		t.Fatalf("%v", err)
	}
}

func TestNetworkErrorsDoNotCarryTheURLQuery(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		http.Redirect(w, r, "http://127.0.0.1:1/signed.csv?sig=SECRETSIG", http.StatusTemporaryRedirect)
	})
	_, err := c.GetFileJobResults(context.Background(), "job-1", nil)
	if err == nil || strings.Contains(err.Error(), "SECRETSIG") || strings.Contains(err.Error(), "signed.csv") {
		t.Fatalf("error leaks the URL: %v", err)
	}
	var be *BillionVerifyError
	if !errors.As(err, &be) || be.Code != "NETWORK_ERROR" {
		t.Fatalf("want NETWORK_ERROR: %v", err)
	}
}

func TestResumeJob(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"task_id":"job-1","status":"processing"}`)
	})
	st, err := c.ResumeJob(context.Background(), "job-1")
	if err != nil || st.Status != JobStatusProcessing || (*reqs)[0].method != "POST" || (*reqs)[0].path != "/v1/jobs/job-1/resume" {
		t.Fatalf("%v %v %+v", st, err, *reqs)
	}
}

func TestUploadFileReaderFields(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		ok(w, `{"task_id":"job-1","status":"pending","email_column_candidates":["email","alt"]}`)
	})
	res, err := c.UploadFileReader(context.Background(), strings.NewReader("email\na@b.co\n"), "emails.csv", &FileUploadOptions{CheckSMTP: false, EmailColumn: "email", PreserveOriginal: true})
	if err != nil || res.TaskID != "job-1" || len(res.EmailColumnCandidates) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	f, name, content := multipartFields(t, (*reqs)[0])
	if name != "emails.csv" || content != "email\na@b.co\n" || f["check_smtp"] != "false" || f["email_column"] != "email" || f["preserve_original"] != "true" {
		t.Errorf("%v %q %q", f, name, content)
	}
}

func TestPreserveOriginalDefaultsToTrue(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { ok(w, `{}`) })
	_, _ = c.UploadFileReader(context.Background(), strings.NewReader("a@b.co"), "x.txt", nil)
	f, _, _ := multipartFields(t, (*reqs)[0])
	if f["preserve_original"] != "true" {
		t.Errorf("%v", f)
	}
}

func TestWebhooksListAndDelete(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		if r.Method == http.MethodDelete {
			ok(w, `{"message":"Webhook deleted successfully"}`)
			return
		}
		ok(w, `{"webhooks":[{"id":"w1","url":"https://x.test/h","events":["file.completed"],"is_active":true,"last_delivery_status":"success","last_error":null}],"total":1}`)
	})
	list, err := c.ListWebhooks(context.Background())
	if err != nil || list.Total != 1 || list.Webhooks[0].LastDeliveryStatus != "success" {
		t.Fatalf("%+v %v", list, err)
	}
	if err := c.DeleteWebhook(context.Background(), "w1"); err != nil || (*reqs)[1].path != "/v1/webhooks/w1" {
		t.Fatalf("%v %+v", err, *reqs)
	}
}

func sign(secret, ts, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "." + body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestVerifyWebhookSignatureWithTimestamp(t *testing.T) {
	const secret = "5c609e6893ab3b65ce8940549a030bc165762fe171e0b38f880bd570099619bf"
	const body = `{"event":"file.completed","data":{"job_id":"j1"}}`
	now := time.Unix(1_700_000_000, 0)
	check := func(sig, ts, sec, b string, tol time.Duration, at time.Time) bool {
		return verifyWebhookSignatureAt(b, sig, ts, sec, tol, at)
	}
	good := sign(secret, "1700000000", body)

	if !check(good, "1700000000", secret, body, 0, now) {
		t.Error("a correct signature must pass")
	}
	if check(good, "1700000000", secret, body+" ", 0, now) || check(good, "1700000000", "other", body, 0, now) || check(good, "1700000001", secret, body, 0, now) {
		t.Error("a changed body, secret or timestamp must fail")
	}
	if check(good, "1700000000", secret, body, 0, now.Add(301*time.Second)) || check(good, "1700000000", secret, body, 0, now.Add(-301*time.Second)) {
		t.Error("old and future timestamps must fail")
	}
	if !check(good, "1700000000", secret, body, 0, now.Add(300*time.Second)) {
		t.Error("the edge of the window passes")
	}
	if !check(good, "1700000000", secret, body, -1, now.Add(10_000_000*time.Second)) {
		t.Error("a negative tolerance skips the time check")
	}
	if !check(strings.TrimPrefix(good, "sha256="), "1700000000", secret, body, 0, now) {
		t.Error("a bare hex digest is accepted")
	}
	for _, ts := range []string{"", "yesterday", "-1", "-9223372036854775808", "9223372036854775808", "1e9"} {
		if check(sign(secret, ts, body), ts, secret, body, 0, now) {
			t.Errorf("timestamp %q must fail", ts)
		}
	}
	// the largest timestamp parses but is far in the future: no overflow, no pass
	if check(sign(secret, "9223372036854775807", body), "9223372036854775807", secret, body, 0, now) {
		t.Error("an extreme future timestamp must fail")
	}
	// an empty or blank secret never verifies, even with a signature made from it
	for _, s := range []string{"", " ", "  "} {
		if check(sign(s, "1700000000", body), "1700000000", s, body, 0, now) {
			t.Errorf("secret %q must fail", s)
		}
	}
	if !VerifyWebhookSignatureWithTimestamp(body, sign(secret, strconv.FormatInt(time.Now().Unix(), 10), body), strconv.FormatInt(time.Now().Unix(), 10), secret, 0) {
		t.Error("the public function uses the current time")
	}
	// the old body-only function is unchanged
	if !VerifyWebhookSignature("p", "sha256="+hexHMAC("s", "p"), "s") {
		t.Error("legacy signature check")
	}
}

func hexHMAC(secret, msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

func TestDomainSearch(t *testing.T) {
	var calls int32
	c, reqs, sleeps := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			ok(w, `{"search_id":"s1","status":"queued","domain":"example.com","credits_reserved":50,"status_url":"/v1/domain/search/s1"}`)
		case atomic.AddInt32(&calls, 1) == 1:
			ok(w, `{"search_id":"s1","status":"running"}`)
		default:
			ok(w, `{"search_id":"s1","status":"completed","credits_charged":5,"result":{"emails":[{"email":"a@example.com"}]}}`)
		}
	})
	ctx := context.Background()
	no := false
	created, err := c.CreateDomainSearch(ctx, "example.com", 10, &DomainSearchOptions{IncludeRoleBased: &no, MaxPages: 50})
	if err != nil || created.SearchID != "s1" || created.CreditsReserved != 50 {
		t.Fatalf("%+v %v", created, err)
	}
	m := bodyMap(t, (*reqs)[0])
	if m["domain"] != "example.com" || m["max_emails"] != float64(10) || m["include_role_based"] != false || m["max_pages"] != float64(50) {
		t.Errorf("body: %v", m)
	}
	done, err := c.WaitForDomainSearch(ctx, "s1", 3*time.Second, time.Minute)
	if err != nil || !done.IsFinished() || done.CreditsCharged != 5 {
		t.Fatalf("%+v %v", done, err)
	}
	emails := done.Result["emails"].([]interface{})
	if emails[0].(map[string]interface{})["email"] != "a@example.com" || len(*sleeps) != 1 || (*sleeps)[0] != 3*time.Second {
		t.Errorf("result/sleeps: %v %v", done.Result, *sleeps)
	}
	for _, n := range []int{0, -1, 201} {
		var ve *ValidationError
		if _, err := c.CreateDomainSearch(ctx, "example.com", n, nil); !errors.As(err, &ve) {
			t.Errorf("max_emails %d must be rejected", n)
		}
	}
	if len(*reqs) != 3 {
		t.Errorf("validation errors must not send a request, got %d requests", len(*reqs))
	}
}

func TestDomainSearchBatch(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			ok(w, `{"batch_id":"b1","status":"queued","total_rows":3,"unique_domains":2,"max_emails":5,"include_role_based":false,"max_pages":20,"status_url":"/s","results_url":"/r"}`)
		case strings.HasSuffix(r.URL.Path, "/results"):
			_, _ = w.Write([]byte("row_no,domain\n1,example.com\n"))
		case r.URL.Path == "/v1/domain/search/batch":
			ok(w, `{"items":[{"batch_id":"b1"}],"has_more":true,"next_cursor":"c2"}`)
		default:
			ok(w, `{"batch_id":"b1","status":"running","progress":{"total_rows":4,"processed":3,"found":1,"emails_returned":2,"unique_domains":4},"credits_charged":15}`)
		}
	})
	ctx := context.Background()
	no := false
	created, err := c.CreateDomainSearchBatchReader(ctx, strings.NewReader("domain\nexample.com\n"), "d.csv",
		&DomainSearchBatchOptions{MaxEmails: 5, IncludeRoleBased: &no, MaxPages: 20, ColumnMap: map[string]string{"domain": "Company Website"}})
	if err != nil || created.BatchID != "b1" || created.UniqueDomains != 2 || created.IncludeRoleBased {
		t.Fatalf("%+v %v", created, err)
	}
	f, name, content := multipartFields(t, (*reqs)[0])
	if name != "d.csv" || content != "domain\nexample.com\n" || f["max_emails"] != "5" || f["include_role_based"] != "false" || f["max_pages"] != "20" || f["column_map"] != `{"domain":"Company Website"}` {
		t.Errorf("%v %q %q", f, name, content)
	}

	page, err := c.ListDomainSearchBatches(ctx, &ListDomainSearchBatchesOptions{Limit: 5, Status: "completed", Cursor: "c1"})
	if err != nil || !page.HasMore || page.NextCursor != "c2" || len(page.Items) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	if (*reqs)[1].query != "cursor=c1&limit=5&status=completed" {
		t.Errorf("query: %s", (*reqs)[1].query)
	}
	st, err := c.GetDomainSearchBatch(ctx, "b1")
	if err != nil || st.Progress.Processed != 3 || st.CreditsCharged != 15 {
		t.Fatalf("%+v %v", st, err)
	}
	csv, err := c.DownloadDomainSearchBatchResults(ctx, "b1")
	if err != nil || string(csv) != "row_no,domain\n1,example.com\n" {
		t.Fatalf("%q %v", csv, err)
	}
	var ve *ValidationError
	if _, err := c.CreateDomainSearchBatchReader(ctx, strings.NewReader("x"), "d.csv", &DomainSearchBatchOptions{MaxEmails: 201}); !errors.As(err, &ve) {
		t.Error("max_emails above 200 must be rejected")
	}
}

func TestBatchResultsNotReadyIs409(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		fail(w, 409, "NOT_READY", "Results are not ready yet")
	})
	_, err := c.DownloadDomainSearchBatchResults(context.Background(), "b1")
	var be *BillionVerifyError
	if !errors.As(err, &be) || be.StatusCode != 409 || be.Code != "NOT_READY" {
		t.Fatalf("%v", err)
	}
}

func TestHealthHasNoEnvelopeAndNoKey(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		_, _ = w.Write([]byte(`{"status":"ok","time":1705319400}`))
	})
	h, err := c.Health(context.Background())
	if err != nil || h.Status != "ok" || h.Time != 1705319400 || (*reqs)[0].header.Get("BV-API-KEY") != "" {
		t.Fatalf("%+v %v %+v", h, err, *reqs)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := map[int]string{400: "*billionverify.ValidationError", 401: "*billionverify.AuthenticationError", 402: "*billionverify.InsufficientCreditsError", 404: "*billionverify.NotFoundError"}
	for status, want := range cases {
		c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { fail(w, status, "X", "boom") })
		_, err := c.GetCredits(context.Background())
		if err == nil || typeName(err) != want {
			t.Errorf("%d: got %T (%v), want %s", status, err, err, want)
		}
	}
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		w.WriteHeader(418)
		_, _ = w.Write([]byte("teapot"))
	})
	_, err := c.GetCredits(context.Background())
	var be *BillionVerifyError
	if !errors.As(err, &be) || be.StatusCode != 418 || be.Code != "UNKNOWN_ERROR" {
		t.Errorf("non-JSON error body: %v", err)
	}
}

func typeName(v interface{}) string { return fmt.Sprintf("%T", v) }

func TestRetries(t *testing.T) {
	var n int32
	c, _, sleeps := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		switch atomic.AddInt32(&n, 1) {
		case 1:
			w.Header().Set("Retry-After", "7")
			fail(w, 429, "RATE_LIMIT_EXCEEDED", "slow")
		case 2:
			fail(w, 503, "X", "down")
		default:
			ok(w, `{"credits_balance":1}`)
		}
	})
	got, err := c.GetCredits(context.Background())
	if err != nil || got.CreditsBalance != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if len(*sleeps) != 2 || (*sleeps)[0] != 7*time.Second || (*sleeps)[1] != 4*time.Second {
		t.Errorf("sleeps: %v", *sleeps)
	}
}

func TestRetriesGiveUp(t *testing.T) {
	c, reqs, sleeps := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) {
		w.Header().Set("Retry-After", "3")
		fail(w, 429, "RATE_LIMIT_EXCEEDED", "slow")
	})
	_, err := c.GetCredits(context.Background())
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 3 || len(*reqs) != 3 || len(*sleeps) != 2 {
		t.Fatalf("%v requests=%d sleeps=%v", err, len(*reqs), *sleeps)
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	c, reqs, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { fail(w, 401, "INVALID_API_KEY", "bad") })
	_, _ = c.GetCredits(context.Background())
	if len(*reqs) != 1 {
		t.Errorf("requests: %d", len(*reqs))
	}
}

func TestContextCancellationIsReturned(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, rec recorded) { ok(w, `{}`) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetCredits(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
}
