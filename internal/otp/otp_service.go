package otp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const otpServiceTracerName = "OtpService"

var ErrInvalidOtp = errors.New("invalid otp")

type Service interface {
	Start(ctx context.Context, email string, purpose EmailOtpPurpose, subject string, textBodyFormat string) error
	VerifyAndConsume(ctx context.Context, email string, rawOtp string, purpose EmailOtpPurpose) error
}

type service struct {
	store EmailOtpRequestStore
}

func NewService(store EmailOtpRequestStore) Service {
	return &service{store: store}
}

func (s *service) Start(ctx context.Context, email string, purpose EmailOtpPurpose, subject string, textBodyFormat string) error {
	const otpLen = 6

	tracer := otel.Tracer(otpServiceTracerName)
	ctx, span := tracer.Start(ctx, "Start")
	defer span.End()

	if err := s.store.RevokeAllPendingEmailOtpRequests(ctx, email, purpose); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke pending otp requests failure")
		return err
	}

	rawOtp, err := generateAlphanumericOTP(otpLen)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate otp failure")
		return err
	}

	record := &EmailOtpRequest{
		Email:     email,
		OtpHash:   hashString(rawOtp),
		Purpose:   purpose,
		Status:    EmailOtpStatusPending,
		ExpiresAt: time.Now().Add(10 * time.Minute).UTC(),
	}

	if err := s.store.CreateEmailOtpRequest(ctx, record); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create otp request failure")
		return err
	}

	return nil
}

func (s *service) VerifyAndConsume(ctx context.Context, email string, rawOtp string, purpose EmailOtpPurpose) error {
	tracer := otel.Tracer(otpServiceTracerName)
	ctx, span := tracer.Start(ctx, "VerifyAndConsume")
	defer span.End()

	record, err := s.store.GetPendingEmailOtpRequest(ctx, email, hashString(rawOtp), purpose)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "fetch pending otp request failure")
		return err
	}

	if record == nil {
		span.SetStatus(codes.Error, "otp not found")
		return ErrInvalidOtp
	}

	if time.Now().UTC().After(record.ExpiresAt) {
		record.Status = EmailOtpStatusExpired
		record.ExpiresAt = time.Now().UTC()
		_ = s.store.UpdateEmailOtpRequest(ctx, record)
		span.SetStatus(codes.Error, "otp expired")
		return ErrInvalidOtp
	}

	record.Status = EmailOtpStatusUsed
	if err = s.store.UpdateEmailOtpRequest(ctx, record); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update otp status failure")
		return err
	}

	return nil
}

func hashString(plainToken string) string {
	h := sha256.New()
	h.Write([]byte(plainToken))
	return hex.EncodeToString(h.Sum(nil))
}

func generateAlphanumericOTP(n int) (string, error) {
	if strings.ToLower(os.Getenv(utils.ApplicationEnvironmentEnvVar)) != "production" {
		return "ABC123", nil
	}

	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, n)
	max := big.NewInt(int64(len(chars)))
	for i := 0; i < n; i++ {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = chars[r.Int64()]
	}
	return string(out), nil
}
