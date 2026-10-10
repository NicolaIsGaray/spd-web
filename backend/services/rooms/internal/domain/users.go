package domain

import "context"

// NewUser son los datos de alta de un usuario.
type NewUser struct {
	FullName string   `json:"full_name"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
}

// UserChanges son los campos modificables de un usuario; los que llegan nil no cambian.
type UserChanges struct {
	FullName *string   `json:"full_name"`
	Email    *string   `json:"email"`
	Password *string   `json:"password"`
	Roles    *[]string `json:"roles"`
}

// CreateUser da de alta un usuario. El correo se normaliza en minúsculas y la contraseña se
// guarda como hash bcrypt.
func (s *Service) CreateUser(ctx context.Context, in NewUser) (User, error) {
	var u User
	var err error
	if u.FullName, err = checkName("full_name", in.FullName); err != nil {
		return User{}, err
	}
	if u.Email, err = checkEmail(in.Email); err != nil {
		return User{}, err
	}
	if u.Roles, err = checkRoles(in.Roles); err != nil {
		return User{}, err
	}
	if u.PasswordHash, err = hashSecret("password", in.Password, minPassword); err != nil {
		return User{}, err
	}
	return s.store.CreateUser(ctx, u)
}

// ListUsers devuelve los usuarios ordenados por id.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	return s.store.ListUsers(ctx)
}

// GetUser devuelve un usuario.
func (s *Service) GetUser(ctx context.Context, id uint) (User, error) {
	return s.store.GetUser(ctx, id)
}

// UpdateUser modifica los campos indicados de un usuario.
func (s *Service) UpdateUser(ctx context.Context, id uint, in UserChanges) (User, error) {
	var ch UserUpdate
	if in.FullName != nil {
		v, err := checkName("full_name", *in.FullName)
		if err != nil {
			return User{}, err
		}
		ch.FullName = &v
	}
	if in.Email != nil {
		v, err := checkEmail(*in.Email)
		if err != nil {
			return User{}, err
		}
		ch.Email = &v
	}
	if in.Roles != nil {
		v, err := checkRoles(*in.Roles)
		if err != nil {
			return User{}, err
		}
		ch.Roles = &v
	}
	if in.Password != nil {
		v, err := hashSecret("password", *in.Password, minPassword)
		if err != nil {
			return User{}, err
		}
		ch.PasswordHash = &v
	}
	return s.store.UpdateUser(ctx, id, ch)
}

// DeleteUser borra (lógicamente) un usuario y sus pertenencias a salas y grupos.
func (s *Service) DeleteUser(ctx context.Context, id uint) error {
	return s.store.DeleteUser(ctx, id)
}

// UserRooms devuelve las salas de las que el usuario es miembro, ordenadas por id.
func (s *Service) UserRooms(ctx context.Context, id uint) ([]Room, error) {
	return s.store.UserRooms(ctx, id)
}

// UserGroups devuelve los grupos de los que el usuario es integrante, por sala y en orden de paso.
func (s *Service) UserGroups(ctx context.Context, id uint) ([]Group, error) {
	return s.store.UserGroups(ctx, id)
}
