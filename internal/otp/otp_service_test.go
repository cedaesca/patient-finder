package otp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- fake store ---

type fakeOtpStore struct {
	createFn     func(ctx context.Context, r *EmailOtpRequest) error
	getByHashFn  func(ctx context.Context, hash string) (*EmailOtpRequest, error)
	getPendingFn func(ctx context.Context, email, hash string, purpose EmailOtpPurpose) (*EmailOtpRequest, error)
	updateFn     func(ctx context.Context, r *EmailOtpRequest) error
	revokeFn     func(ctx context.Context, email string, purpose EmailOtpPurpose) error
}

func (f *fakeOtpStore) CreateEmailOtpRequest(ctx context.Context, r *EmailOtpRequest) error {
	if f.createFn != nil {
		return f.createFn(ctx, r)
	}
	return nil
}

func (f *fakeOtpStore) GetEmailOtpRequestByHash(ctx context.Context, hash string) (*EmailOtpRequest, error) {
	if f.getByHashFn != nil {
		return f.getByHashFn(ctx, hash)
	}
	return nil, nil
}

func (f *fakeOtpStore) GetPendingEmailOtpRequest(ctx context.Context, email, hash string, purpose EmailOtpPurpose) (*EmailOtpRequest, error) {
	if f.getPendingFn != nil {
		return f.getPendingFn(ctx, email, hash, purpose)
	}
	return nil, nil
}

func (f *fakeOtpStore) UpdateEmailOtpRequest(ctx context.Context, r *EmailOtpRequest) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, r)
	}
	return nil
}

func (f *fakeOtpStore) RevokeAllPendingEmailOtpRequests(ctx context.Context, email string, purpose EmailOtpPurpose) error {
	if f.revokeFn != nil {
		return f.revokeFn(ctx, email, purpose)
	}
	return nil
}

// --- hashString ---

func TestHashString_Deterministic(t *testing.T) {
	h1 := hashString("ABC123")
	h2 := hashString("ABC123")
	if h1 != h2 {
		t.Fatalf("expected deterministic hash; got %q vs %q", h1, h2)
	}
	if h1 == "ABC123" {
		t.Fatalf("hash must not equal plaintext")
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %d", len(h1))
	}
}

func TestHashString_DifferentInputs(t *testing.T) {
	if hashString("a") == hashString("b") {
		t.Fatalf("expected different hashes for different inputs")
	}
}

// --- generateAlphanumericOTP ---

func TestGenerateAlphanumericOTP_NonProdReturnsFixed(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	otp, err := generateAlphanumericOTP(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if otp != "ABC123" {
		t.Fatalf("expected fixed OTP in non-prod, got %q", otp)
	}
}

func TestGenerateAlphanumericOTP_ProdReturnsRandomFromAlphabet(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	otp, err := generateAlphanumericOTP(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(otp) != 6 {
		t.Fatalf("expected length 6, got %d", len(otp))
	}
	for _, c := range otp {
		if !strings.ContainsRune(alphabet, c) {
			t.Fatalf("OTP contains char %q outside alphabet", c)
		}
	}

	// Two draws should almost certainly differ.
	other, _ := generateAlphanumericOTP(6)
	if otp == other {
		t.Fatalf("two consecutive prod OTPs matched, very unlikely: %q", otp)
	}
}

// --- Start ---

func TestStart_RevokesThenCreates(t *testing.T) {
	revoked := make(chan struct{}, 1)
	created := make(chan *EmailOtpRequest, 1)

	store := &fakeOtpStore{
		revokeFn: func(context.Context, string, EmailOtpPurpose) error {
			revoked <- struct{}{}
			return nil
		},
		createFn: func(_ context.Context, r *EmailOtpRequest) error {
			created <- r
			return nil
		},
	}

	svc := NewService(store)
	err := svc.Start(context.Background(), "user@example.com", EmailOtpPurposePasswordChange, "Subject", "Your code: %s")
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("Revoke never called")
	}

	var record *EmailOtpRequest
	select {
	case record = <-created:
	case <-time.After(time.Second):
		t.Fatal("Create never called")
	}

	if record.Email != "user@example.com" {
		t.Fatalf("email mismatch: %q", record.Email)
	}
	if record.Status != EmailOtpStatusPending {
		t.Fatalf("expected Pending, got %q", record.Status)
	}
	if record.OtpHash == "" {
		t.Fatalf("otp hash not set")
	}
	if !record.ExpiresAt.After(time.Now()) {
		t.Fatalf("ExpiresAt should be in the future: %v", record.ExpiresAt)
	}
}

func TestStart_RevokeErrorShortCircuits(t *testing.T) {
	store := &fakeOtpStore{
		revokeFn: func(context.Context, string, EmailOtpPurpose) error {
			return errors.New("revoke failed")
		},
		createFn: func(context.Context, *EmailOtpRequest) error {
			t.Fatal("Create should not be called when Revoke fails")
			return nil
		},
	}
	svc := NewService(store)

	err := svc.Start(context.Background(), "user@example.com", EmailOtpPurposePasswordChange, "S", "%s")
	if err == nil {
		t.Fatal("expected error from Start")
	}
}

// --- VerifyAndConsume ---

func TestVerifyAndConsume_ValidOtpMarksUsed(t *testing.T) {
	record := &EmailOtpRequest{
		ID:        uuid.New(),
		Email:     "u@e.com",
		OtpHash:   hashString("ABC123"),
		Purpose:   EmailOtpPurposePasswordChange,
		Status:    EmailOtpStatusPending,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	var updated *EmailOtpRequest
	store := &fakeOtpStore{
		getPendingFn: func(context.Context, string, string, EmailOtpPurpose) (*EmailOtpRequest, error) {
			return record, nil
		},
		updateFn: func(_ context.Context, r *EmailOtpRequest) error {
			updated = r
			return nil
		},
	}

	svc := NewService(store)
	err := svc.VerifyAndConsume(context.Background(), "u@e.com", "ABC123", EmailOtpPurposePasswordChange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated == nil || updated.Status != EmailOtpStatusUsed {
		t.Fatalf("expected status=used, got %+v", updated)
	}
}

func TestVerifyAndConsume_NotFoundReturnsInvalid(t *testing.T) {
	store := &fakeOtpStore{
		getPendingFn: func(context.Context, string, string, EmailOtpPurpose) (*EmailOtpRequest, error) {
			return nil, nil
		},
	}
	svc := NewService(store)
	err := svc.VerifyAndConsume(context.Background(), "u@e.com", "x", EmailOtpPurposePasswordChange)
	if !errors.Is(err, ErrInvalidOtp) {
		t.Fatalf("expected ErrInvalidOtp, got %v", err)
	}
}

func TestVerifyAndConsume_ExpiredReturnsInvalid(t *testing.T) {
	record := &EmailOtpRequest{
		ID:        uuid.New(),
		Email:     "u@e.com",
		OtpHash:   hashString("ABC123"),
		Purpose:   EmailOtpPurposePasswordChange,
		Status:    EmailOtpStatusPending,
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	var updated *EmailOtpRequest
	store := &fakeOtpStore{
		getPendingFn: func(context.Context, string, string, EmailOtpPurpose) (*EmailOtpRequest, error) {
			return record, nil
		},
		updateFn: func(_ context.Context, r *EmailOtpRequest) error {
			updated = r
			return nil
		},
	}
	svc := NewService(store)
	err := svc.VerifyAndConsume(context.Background(), "u@e.com", "ABC123", EmailOtpPurposePasswordChange)
	if !errors.Is(err, ErrInvalidOtp) {
		t.Fatalf("expected ErrInvalidOtp, got %v", err)
	}
	if updated == nil || updated.Status != EmailOtpStatusExpired {
		t.Fatalf("expected record to be marked expired, got %+v", updated)
	}
}

func TestVerifyAndConsume_StoreErrorPropagates(t *testing.T) {
	store := &fakeOtpStore{
		getPendingFn: func(context.Context, string, string, EmailOtpPurpose) (*EmailOtpRequest, error) {
			return nil, errors.New("db dead")
		},
	}
	svc := NewService(store)
	err := svc.VerifyAndConsume(context.Background(), "u@e.com", "x", EmailOtpPurposePasswordChange)
	if err == nil || errors.Is(err, ErrInvalidOtp) {
		t.Fatalf("expected non-invalid error, got %v", err)
	}
}
