package controlapp

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"strings"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	controlport "github.com/gsoultan/anubis/internal/control/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/platform/crypto/totp"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// PlatformAudience is what a platform token is minted for. It is not a tenant
// application, and a tenant's verifier must never accept it: the audience is
// what keeps an operator's token from being replayed at a relying party.
const PlatformAudience = "anubis-platform"

// platformTokenTTL is short because there is no refresh yet — the console
// asks for the password again rather than holding a long-lived operator
// token, which is the safer half of that trade.
const platformTokenTTL = time.Hour

// PlatformAuthUsecase signs platform users in.
//
// This is a DIFFERENT door from tenant sign-in. A tenant's people authenticate
// through their realm, against identities, and may be sent to a branded page
// built for them; an operator authenticates here, against platform_users,
// with no tenant involved at all. Neither can use the other's door.
type PlatformAuthUsecase interface {
	// Login returns a session, or a challenge when the operator has a second
	// factor enrolled.
	Login(ctx context.Context, username, password string) (*PlatformSession, error)
	// VerifyMFA completes a challenge.
	VerifyMFA(ctx context.Context, mfaToken, code string) (*PlatformSession, error)
	// BeginTOTPEnrolment issues a secret for the signed-in operator. Nothing
	// is demanded of them until ConfirmTOTPEnrolment verifies a code.
	BeginTOTPEnrolment(ctx context.Context) (secretB32, uri string, err error)
	ConfirmTOTPEnrolment(ctx context.Context, code string) error
	// MyTenants is which tenants the caller may administer: the ones they
	// are assigned to, or every tenant when they own the installation.
	MyTenants(ctx context.Context) ([]TenantChoice, error)
}

// TenantChoice is one entry in the header's tenant picker.
type TenantChoice struct {
	Slug string
	Name string
	Role string
	// All marks an owner's blanket authority rather than a listed assignment.
	All bool
}

// PlatformSession is what an operator sign-in yields: either a token, or a
// challenge when a second factor is enrolled.
type PlatformSession struct {
	AccessToken string
	ExpiresIn   int
	Username    string
	Owner       bool
	// MFAToken is set when a second factor is required. AccessToken is empty
	// then: a password alone is not a session for these accounts.
	MFAToken string
}

// mfaChallengeTTL is short. It exists to carry one verification, not to be a
// session in its own right.
const mfaChallengeTTL = 3 * time.Minute

// mfaState is what a challenge carries. Sealed, never readable by the browser.
type mfaState struct {
	OperatorID string `json:"oid"`
}

type platformAuthInteractor struct {
	users   controlport.PlatformUserStore
	read    controlport.AssignmentReader
	tenants controlport.TenantLookup
	ring    *keyring.Manager
	// master seals TOTP secrets at rest, bound to each operator's row id.
	master []byte
	clock  clock.Clock
	audit  auditport.Auditor
	issuer string
}

func NewPlatformAuthInteractor(
	users controlport.PlatformUserStore,
	read controlport.AssignmentReader,
	tenants controlport.TenantLookup,
	ring *keyring.Manager,
	clk clock.Clock,
	audit auditport.Auditor,
	issuer string,
	master []byte,
) PlatformAuthUsecase {
	return &platformAuthInteractor{users: users, read: read, tenants: tenants,
		ring: ring, clock: clk, audit: audit, issuer: issuer, master: master}
}

func (u *platformAuthInteractor) Login(ctx context.Context, username, password string) (*PlatformSession, error) {
	username = strings.TrimSpace(username)
	who, hash, err := u.users.PlatformUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	// Verify even when nobody was found, so a missing account and a wrong
	// password cost the same time and cannot be told apart from outside.
	ok, _, _ := kdf.Verify(password, hash)
	if who == nil || !ok {
		u.deny(ctx, username, "invalid_credentials")
		return nil, apperr.ErrInvalidCredentials
	}
	if !who.Active() {
		u.deny(ctx, username, "disabled")
		return nil, apperr.ErrIdentityDisabled
	}

	assignments, err := u.read.Assignments(ctx)
	if err != nil {
		return nil, err
	}
	now := u.clock.Now()
	roles := make([]string, 0, 4)
	live := false
	for _, a := range assignments {
		if a.OperatorID != who.ID || !a.Live(now) {
			continue
		}
		live = true
		roles = append(roles, string(a.Role))
		if a.Global() && a.Role == "owner" {
			who.Assignments = append(who.Assignments, a)
		}
	}
	if !live {
		// Signing somebody in who can reach nothing is a worse experience
		// than telling them their access has been removed.
		u.deny(ctx, username, "no_assignment")
		return nil, apperr.ErrPermissionDenied.With("operator", "no live assignment")
	}

	// An enrolled factor is always demanded. There is deliberately no
	// "required but not enrolled" flip: turning that on would lock out every
	// operator who had not yet enrolled, including the only owner.
	if who.MFAEnrolled() {
		challenge, cerr := u.challenge(who.ID, now)
		if cerr != nil {
			return nil, cerr
		}
		return &PlatformSession{Username: who.Username, MFAToken: challenge}, nil
	}

	token, err := u.mint(who.ID, roles, who.TokenEpoch, now)
	if err != nil {
		return nil, err
	}
	u.users.TouchLogin(ctx, who.ID)
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		ActorID: who.ID, ActorKind: "platform_user", TargetID: who.ID,
		Action: "platform.login", Result: "allow", IP: authctx.ClientIP(ctx),
	})
	return &PlatformSession{
		AccessToken: token,
		ExpiresIn:   int(platformTokenTTL.Seconds()),
		Username:    who.Username,
		Owner:       who.Owner(now),
	}, nil
}

// mint signs a platform token. Tenant is deliberately empty: an operator
// belongs to none, and the interceptor uses that plus the audience to decide
// it is looking at a platform principal rather than a tenant one.
func (u *platformAuthInteractor) mint(subject string, roles []string, epoch int, now time.Time) (string, error) {
	return u.signClaims(anubis.Claims{
		Issuer:    u.issuer,
		Subject:   subject,
		Audience:  []string{PlatformAudience},
		Expires:   now.Add(platformTokenTTL).Unix(),
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		Roles:     roles,
		Epoch:     epoch,
		Version:   1,
	})
}

func (u *platformAuthInteractor) signClaims(claims anubis.Claims) (string, error) {
	body, err := json.Marshal(claims)
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	key, err := u.ring.Ring().ActiveAccess()
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	footer, _ := json.Marshal(map[string]string{"kid": key.Kid})
	token, err := paseto.Sign(key.Private, body, footer, nil)
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	return token, nil
}

func (u *platformAuthInteractor) deny(ctx context.Context, username, reason string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		ActorKind: "platform_user", TargetID: username,
		Action: "platform.login", Result: "deny", IP: authctx.ClientIP(ctx),
		Detail: []byte(`{"reason":"` + reason + `"}`),
	})
}

// live returns the caller's assignments still in force.
func (u *platformAuthInteractor) live(ctx context.Context) (*authctx.Principal, []controldomain.AssignmentRecord, error) {
	p, ok := authctx.From(ctx)
	if !ok || !p.Platform {
		return nil, nil, apperr.ErrUnauthenticated
	}
	all, err := u.read.Assignments(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := u.clock.Now()
	mine := make([]controldomain.AssignmentRecord, 0, 4)
	for _, a := range all {
		if a.OperatorID == p.IdentityID && a.Live(now) {
			mine = append(mine, a)
		}
	}
	return p, mine, nil
}

func (u *platformAuthInteractor) MyTenants(ctx context.Context) ([]TenantChoice, error) {
	_, mine, err := u.live(ctx)
	if err != nil {
		return nil, err
	}
	// An owner's authority is not a list of tenants, so it has to be
	// expanded into one before a picker can show it.
	for _, a := range mine {
		if a.Global() {
			all, lerr := u.tenants.ListTenants(ctx)
			if lerr != nil {
				return nil, lerr
			}
			out := make([]TenantChoice, 0, len(all))
			for _, t := range all {
				out = append(out, TenantChoice{Slug: t.Slug, Name: t.Name, Role: string(a.Role), All: true})
			}
			return out, nil
		}
	}
	out := make([]TenantChoice, 0, len(mine))
	for _, a := range mine {
		t, terr := u.tenants.TenantBySlug(ctx, a.TenantSlug)
		if terr != nil || t == nil {
			continue
		}
		out = append(out, TenantChoice{Slug: t.Slug, Name: t.Name, Role: string(a.Role)})
	}
	return out, nil
}

// challenge seals a short-lived token naming the operator who got past the
// password. It carries no authority of its own — only VerifyMFA can turn it
// into a session, and only with a code.
func (u *platformAuthInteractor) challenge(operatorID string, now time.Time) (string, error) {
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	token, err := localtoken.Seal(key.Secret, key.Kid, purposePlatformMFA, jti,
		mfaState{OperatorID: operatorID}, mfaChallengeTTL, now)
	if err != nil {
		return "", apperr.ErrInternal.Wrap(err)
	}
	return token, nil
}

const purposePlatformMFA = "platform_mfa"

// VerifyMFA completes a challenge.
func (u *platformAuthInteractor) VerifyMFA(ctx context.Context, mfaToken, code string) (*PlatformSession, error) {
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	_, raw, err := localtoken.Open(key.Secret, mfaToken, purposePlatformMFA, u.clock.Now())
	if err != nil {
		return nil, apperr.ErrInvalidCredentials
	}
	var st mfaState
	if json.Unmarshal(raw, &st) != nil || st.OperatorID == "" {
		return nil, apperr.ErrInvalidCredentials
	}

	who, _, err := u.users.PlatformUserByID(ctx, st.OperatorID)
	if err != nil || who == nil || !who.Active() || !who.MFAEnrolled() {
		return nil, apperr.ErrInvalidCredentials
	}
	sec, err := u.users.TOTPSecret(ctx, u.master, who.ID)
	if err != nil || len(sec) == 0 {
		return nil, apperr.ErrInvalidCredentials
	}

	now := u.clock.Now()
	step, ok := totp.Verify(sec, code, now, 30*time.Second, 6, 1)
	if !ok {
		u.deny(ctx, who.Username, "bad_totp")
		return nil, apperr.ErrInvalidCredentials
	}
	// Single use: the step must be strictly newer than the last accepted, so
	// a code observed in flight cannot be replayed inside its own window.
	fresh, err := u.users.AdvanceTOTPStep(ctx, who.ID, step)
	if err != nil {
		return nil, err
	}
	if !fresh {
		u.deny(ctx, who.Username, "totp_replay")
		return nil, apperr.ErrInvalidCredentials
	}

	roles, live, err := u.liveRoles(ctx, who.ID, now)
	if err != nil {
		return nil, err
	}
	if !live {
		return nil, apperr.ErrPermissionDenied.With("operator", "no live assignment")
	}
	token, err := u.mint(who.ID, roles, who.TokenEpoch, now)
	if err != nil {
		return nil, err
	}
	u.users.TouchLogin(ctx, who.ID)
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		ActorID: who.ID, ActorKind: "platform_user", TargetID: who.ID,
		Action: "platform.login", Result: "allow", IP: authctx.ClientIP(ctx),
		Detail: []byte(`{"amr":"totp"}`),
	})
	return &PlatformSession{
		AccessToken: token, ExpiresIn: int(platformTokenTTL.Seconds()),
		Username: who.Username, Owner: who.Owner(now),
	}, nil
}

// liveRoles is the operator's roles from assignments still in force.
func (u *platformAuthInteractor) liveRoles(ctx context.Context, operatorID string, now time.Time) ([]string, bool, error) {
	all, err := u.read.Assignments(ctx)
	if err != nil {
		return nil, false, err
	}
	roles := make([]string, 0, 4)
	for _, a := range all {
		if a.OperatorID == operatorID && a.Live(now) {
			roles = append(roles, string(a.Role))
		}
	}
	return roles, len(roles) > 0, nil
}

// BeginTOTPEnrolment issues a secret for the signed-in operator. Nothing is
// demanded of them until a code confirms it: a half-finished enrolment must
// never start requiring a factor they cannot produce.
func (u *platformAuthInteractor) BeginTOTPEnrolment(ctx context.Context) (string, string, error) {
	p, ok := authctx.From(ctx)
	if !ok || !p.Platform {
		return "", "", apperr.ErrUnauthenticated
	}
	who, _, err := u.users.PlatformUserByID(ctx, p.IdentityID)
	if err != nil || who == nil {
		return "", "", apperr.ErrNotFound
	}
	sec, err := totp.NewSecret()
	if err != nil {
		return "", "", apperr.ErrInternal.Wrap(err)
	}
	if err := u.users.StageTOTPSecret(ctx, u.master, who.ID, sec); err != nil {
		return "", "", err
	}
	uri := totp.ProvisioningURI(sec, "Anubis", who.Username, 6, 30*time.Second)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sec), uri, nil
}

// ConfirmTOTPEnrolment verifies the first code and turns the factor on.
func (u *platformAuthInteractor) ConfirmTOTPEnrolment(ctx context.Context, code string) error {
	p, ok := authctx.From(ctx)
	if !ok || !p.Platform {
		return apperr.ErrUnauthenticated
	}
	sec, err := u.users.TOTPSecret(ctx, u.master, p.IdentityID)
	if err != nil {
		return err
	}
	if len(sec) == 0 {
		return apperr.ErrInvalidArgument.With("totp", "no enrolment in progress")
	}
	step, ok := totp.Verify(sec, code, u.clock.Now(), 30*time.Second, 6, 1)
	if !ok {
		return apperr.ErrInvalidCredentials
	}
	if err := u.users.ConfirmTOTP(ctx, p.IdentityID, step); err != nil {
		return err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		ActorID: p.IdentityID, ActorKind: "platform_user", TargetID: p.IdentityID,
		Action: "platform.mfa_enrol", Result: "allow", IP: authctx.ClientIP(ctx),
	})
	return nil
}
