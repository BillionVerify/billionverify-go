# Changelog

## 1.1.0

- Requests authenticate with the `BV-API-KEY` header (the API's recommended name) instead of `EV-API-KEY`.
- New operations: `CheckDisposable`, `CheckDisposableViaQuery`, `ResumeJob`, `CreateDomainSearch`, `GetDomainSearch`, `WaitForDomainSearch`, `CreateDomainSearchBatch`, `CreateDomainSearchBatchReader`, `ListDomainSearchBatches`, `GetDomainSearchBatch`, `DownloadDomainSearchBatchResults`.
- `FileJobResponse` follows the API: `TaskID`, `Progress`, `Stage`, `DeliverabilityBuckets`, `RiskyEmails`, download and runtime fields. `JobID` and `ProgressPercent` are kept and filled from the same values (they used to read as empty and 0).
- All ten job statuses are defined; `WaitForFileJobCompletion` returns on `completed`, `completed_with_warning`, `failed`, `cancelled` and `paused` instead of polling until the limit.
- `VerifyWebhookSignatureWithTimestamp` checks the signature over `<timestamp>.<body>` with a 5 minute window; an empty secret never verifies. `VerifyWebhookSignature` (body only) is kept and documented as not matching current deliveries.
- `VerifyOptions` and `BatchVerifyOptions` gain `ForceRefresh`, `IncludeDomainReputation` and `ServerDefaultSMTP`.
- Batch results gain `DeliverabilityBuckets`, `RetryableEmails`, `DeferredEmails` and `RetryAfterSeconds`; per-row fields the API leaves out are optional.
- `VerifyResponse` gains the current result fields (`VerificationMode`, `Retryable`, `RiskReasons`, ...); `CheckedSMTP` holds `check_smtp`, and `SMTPCheck` is filled from it.
- Long polling (`FileJobStatusOptions.Timeout`) is no longer cut off by the client's own timeout.
- Result downloads send the API key only to the API's own origin; error messages carry no request URL.
- A base URL ending in `/v1` is accepted.
- Cancelling the context returns the context's error.
