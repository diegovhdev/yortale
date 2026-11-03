package moderation

import (
	"strings"
	"testing"

	"github.com/diegovhdev/yortale/internal/auth"
)

func TestPermissions(t *testing.T) {
	user := auth.User{ID: 1, Role: auth.RoleUser}
	user2 := auth.User{ID: 2, Role: auth.RoleUser}
	mod := auth.User{ID: 3, Role: auth.RoleModerator}
	mod2 := auth.User{ID: 4, Role: auth.RoleModerator}
	admin := auth.User{ID: 5, Role: auth.RoleAdmin}
	admin2 := auth.User{ID: 6, Role: auth.RoleAdmin}

	for _, tc := range []struct {
		name          string
		actor, target auth.User
		suspend, role bool
	}{
		{"usuario a usuario", user, user2, false, false},
		{"moderador a usuario", mod, user, true, false},
		{"moderador a moderador", mod, mod2, false, false},
		{"moderador a administrador", mod, admin, false, false},
		{"moderador a sí mismo", mod, mod, false, false},
		{"administrador a usuario", admin, user, true, true},
		{"administrador a moderador", admin, mod, true, true},
		{"administrador a administrador", admin, admin2, false, true},
		{"administrador a sí mismo", admin, admin, false, false},
	} {
		if got := canSuspend(tc.actor, tc.target); got != tc.suspend {
			t.Errorf("%s: canSuspend = %v, se esperaba %v", tc.name, got, tc.suspend)
		}
		if got := canChangeRole(tc.actor, tc.target); got != tc.role {
			t.Errorf("%s: canChangeRole = %v, se esperaba %v", tc.name, got, tc.role)
		}
	}
}

func TestDurationFor(t *testing.T) {
	mod := auth.User{Role: auth.RoleModerator}
	admin := auth.User{Role: auth.RoleAdmin}

	if d, ok := durationFor(mod, "7"); !ok || d.Days != 7 {
		t.Errorf("moderador, 7 días: %+v, %v", d, ok)
	}
	if _, ok := durationFor(mod, durationPermanent); ok {
		t.Error("un moderador no debería poder suspender de forma indefinida")
	}
	if d, ok := durationFor(admin, durationPermanent); !ok || d.Days != 0 {
		t.Errorf("administrador, indefinida: %+v, %v", d, ok)
	}
	if _, ok := durationFor(admin, "365"); ok {
		t.Error("una duración que no está en la lista no debería valer")
	}
	if n := len(durationsFor(mod)); n != len(suspensionDurations)-1 {
		t.Errorf("un moderador ve %d duraciones", n)
	}
}

func TestCleanModReason(t *testing.T) {
	if r, msg := cleanModReason("  Plagio\r\nconfirmado ", "vacío"); r != "Plagio\nconfirmado" || msg != "" {
		t.Errorf("motivo limpio = %q, %q", r, msg)
	}
	if _, msg := cleanModReason(" \n ", "vacío"); msg != "vacío" {
		t.Errorf("motivo vacío: mensaje %q", msg)
	}
	if _, msg := cleanModReason(strings.Repeat("ñ", maxModReasonRunes+1), "vacío"); msg == "" {
		t.Error("un motivo demasiado largo debería dar error")
	}
}
