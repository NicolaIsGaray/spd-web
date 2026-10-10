package domain

import (
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	maxNameLen   = 200  // caracteres de full_name y del nombre de una sala
	maxEmailLen  = 254  // límite práctico de una dirección de correo
	maxURLLen    = 2048 // caracteres
	maxRoles     = 32
	maxRoleLen   = 64 // caracteres
	minPassword  = 8  // caracteres
	minAccessKey = 4  // caracteres
	maxSecret    = 72 // bytes: bcrypt no admite más
)

// invalid crea un error ErrInvalid con el detalle indicado.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// checkName valida un nombre obligatorio (sin espacios a los lados).
func checkName(field, v string) (string, error) {
	v = strings.TrimSpace(v)
	switch n := utf8.RuneCountInString(v); {
	case n == 0:
		return "", invalid("%s es obligatorio", field)
	case n > maxNameLen:
		return "", invalid("%s admite como mucho %d caracteres", field, maxNameLen)
	}
	return v, nil
}

// checkEmail valida una dirección de correo simple (sin nombre visible) y la normaliza en
// minúsculas, para que la unicidad no dependa de mayúsculas.
func checkEmail(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", invalid("email es obligatorio")
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || len(v) > maxEmailLen {
		return "", invalid("email no es un correo válido")
	}
	return v, nil
}

// checkRoles limpia los roles (sin espacios a los lados ni repetidos, en el orden dado).
func checkRoles(roles []string) ([]string, error) {
	if len(roles) > maxRoles {
		return nil, invalid("roles admite como mucho %d valores", maxRoles)
	}
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		r = strings.TrimSpace(r)
		switch {
		case r == "":
			return nil, invalid("roles no admite valores vacíos")
		case utf8.RuneCountInString(r) > maxRoleLen:
			return nil, invalid("cada rol admite como mucho %d caracteres", maxRoleLen)
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// checkURL valida una URL opcional: vacía, o absoluta http(s).
func checkURL(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(v) > maxURLLen {
		return "", invalid("url debe ser una URL absoluta http(s) de como mucho %d caracteres", maxURLLen)
	}
	return v, nil
}

// hashSecret valida la longitud de una contraseña o clave y devuelve su hash bcrypt. El valor
// se usa tal cual, sin recortar espacios.
func hashSecret(field, v string, minLen int) (string, error) {
	switch {
	case utf8.RuneCountInString(v) < minLen:
		return "", invalid("%s debe tener al menos %d caracteres", field, minLen)
	case len(v) > maxSecret:
		return "", invalid("%s admite como mucho %d bytes", field, maxSecret)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(v), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// accessKeyHash devuelve el hash de una clave de ingreso, o "" (sala sin clave) si está vacía.
func accessKeyHash(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return hashSecret("access_key", v, minAccessKey)
}

// checkGroup valida los campos de un grupo que no son nil: el cupo (limit) y la prioridad de
// paso al presentar (priority, 1 presenta primero) son números de 1 en adelante.
func checkGroup(in GroupFields) error {
	switch {
	case in.Limit != nil && *in.Limit < 1:
		return invalid("limit es el cupo del grupo: debe ser 1 o más")
	case in.Priority != nil && *in.Priority < 1:
		return invalid("priority es la prioridad de paso al presentar: debe ser 1 o más (1 presenta primero)")
	}
	return nil
}
