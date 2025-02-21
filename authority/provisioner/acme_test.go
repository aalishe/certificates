package provisioner

import (
	"context"
	"crypto/x509"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/smallstep/assert"
)

func TestACME_Getters(t *testing.T) {
	p, err := generateACME()
	assert.FatalError(t, err)
	id := "acme/" + p.Name
	if got := p.GetID(); got != id {
		t.Errorf("ACME.GetID() = %v, want %v", got, id)
	}
	if got := p.GetName(); got != p.Name {
		t.Errorf("ACME.GetName() = %v, want %v", got, p.Name)
	}
	if got := p.GetType(); got != TypeACME {
		t.Errorf("ACME.GetType() = %v, want %v", got, TypeACME)
	}
	kid, key, ok := p.GetEncryptedKey()
	if kid != "" || key != "" || ok == true {
		t.Errorf("ACME.GetEncryptedKey() = (%v, %v, %v), want (%v, %v, %v)",
			kid, key, ok, "", "", false)
	}
}

func TestACME_Init(t *testing.T) {
	type ProvisionerValidateTest struct {
		p   *ACME
		err error
	}
	tests := map[string]func(*testing.T) ProvisionerValidateTest{
		"fail-empty": func(t *testing.T) ProvisionerValidateTest {
			return ProvisionerValidateTest{
				p:   &ACME{},
				err: errors.New("provisioner type cannot be empty"),
			}
		},
		"fail-empty-name": func(t *testing.T) ProvisionerValidateTest {
			return ProvisionerValidateTest{
				p: &ACME{
					Type: "ACME",
				},
				err: errors.New("provisioner name cannot be empty"),
			}
		},
		"fail-empty-type": func(t *testing.T) ProvisionerValidateTest {
			return ProvisionerValidateTest{
				p:   &ACME{Name: "foo"},
				err: errors.New("provisioner type cannot be empty"),
			}
		},
		"fail-bad-claims": func(t *testing.T) ProvisionerValidateTest {
			return ProvisionerValidateTest{
				p:   &ACME{Name: "foo", Type: "bar", Claims: &Claims{DefaultTLSDur: &Duration{0}}},
				err: errors.New("claims: DefaultTLSCertDuration must be greater than 0"),
			}
		},
		"ok": func(t *testing.T) ProvisionerValidateTest {
			return ProvisionerValidateTest{
				p: &ACME{Name: "foo", Type: "bar"},
			}
		},
	}

	config := Config{
		Claims:    globalProvisionerClaims,
		Audiences: testAudiences,
	}
	for name, get := range tests {
		t.Run(name, func(t *testing.T) {
			tc := get(t)
			err := tc.p.Init(config)
			if err != nil {
				if assert.NotNil(t, tc.err) {
					assert.Equals(t, tc.err.Error(), err.Error())
				}
			} else {
				assert.Nil(t, tc.err)
			}
		})
	}
}

func TestACME_AuthorizeRevoke(t *testing.T) {
	p, err := generateACME()
	assert.FatalError(t, err)
	assert.Nil(t, p.AuthorizeRevoke(""))
}

func TestACME_AuthorizeRenewal(t *testing.T) {
	p1, err := generateACME()
	assert.FatalError(t, err)
	p2, err := generateACME()
	assert.FatalError(t, err)

	// disable renewal
	disable := true
	p2.Claims = &Claims{DisableRenewal: &disable}
	p2.claimer, err = NewClaimer(p2.Claims, globalProvisionerClaims)
	assert.FatalError(t, err)

	type args struct {
		cert *x509.Certificate
	}
	tests := []struct {
		name string
		prov *ACME
		args args
		err  error
	}{
		{"ok", p1, args{nil}, nil},
		{"fail", p2, args{nil}, errors.Errorf("renew is disabled for provisioner %s", p2.GetID())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.prov.AuthorizeRenewal(tt.args.cert); err != nil {
				if assert.NotNil(t, tt.err) {
					assert.HasPrefix(t, err.Error(), tt.err.Error())
				}
			} else {
				assert.Nil(t, tt.err)
			}
		})
	}
}

func TestACME_AuthorizeSign(t *testing.T) {
	p1, err := generateACME()
	assert.FatalError(t, err)

	tests := []struct {
		name   string
		prov   *ACME
		method Method
		err    error
	}{
		{"fail/method", p1, SignSSHMethod, errors.New("unexpected method type 1 in context")},
		{"ok", p1, SignMethod, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContextWithMethod(context.Background(), tt.method)
			if got, err := tt.prov.AuthorizeSign(ctx, ""); err != nil {
				if assert.NotNil(t, tt.err) {
					assert.HasPrefix(t, err.Error(), tt.err.Error())
				}
			} else {
				if assert.NotNil(t, got) {
					assert.Len(t, 4, got)

					_pdd := got[0]
					pdd, ok := _pdd.(profileDefaultDuration)
					assert.True(t, ok)
					assert.Equals(t, pdd, profileDefaultDuration(86400000000000))

					_peo := got[1]
					peo, ok := _peo.(*provisionerExtensionOption)
					assert.True(t, ok)
					assert.Equals(t, peo.Type, 6)
					assert.Equals(t, peo.Name, "test@acme-provisioner.com")
					assert.Equals(t, peo.CredentialID, "")
					assert.Equals(t, peo.KeyValuePairs, nil)

					_vv := got[2]
					vv, ok := _vv.(*validityValidator)
					assert.True(t, ok)
					assert.Equals(t, vv.min, time.Duration(300000000000))
					assert.Equals(t, vv.max, time.Duration(86400000000000))

					_dpkv := got[3]
					_, ok = _dpkv.(defaultPublicKeyValidator)
					assert.True(t, ok)
				}
			}
		})
	}
}
