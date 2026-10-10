package domain

import (
	"context"

	"golang.org/x/crypto/bcrypt"
)

// NewRoom son los datos de alta de una sala. AccessKey vacía: sala sin clave.
type NewRoom struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	AccessKey string `json:"access_key"`
}

// RoomChanges son los campos modificables de una sala; los que llegan nil no cambian. Una
// AccessKey vacía quita la clave.
type RoomChanges struct {
	Name      *string `json:"name"`
	URL       *string `json:"url"`
	AccessKey *string `json:"access_key"`
}

// Join son los datos para unirse a una sala.
type Join struct {
	UserID    uint   `json:"user_id"`
	AccessKey string `json:"access_key"`
}

// CreateRoom da de alta una sala. La clave de ingreso se guarda como hash bcrypt.
func (s *Service) CreateRoom(ctx context.Context, in NewRoom) (Room, error) {
	var r Room
	var err error
	if r.Name, err = checkName("name", in.Name); err != nil {
		return Room{}, err
	}
	if r.URL, err = checkURL(in.URL); err != nil {
		return Room{}, err
	}
	if r.AccessKeyHash, err = accessKeyHash(in.AccessKey); err != nil {
		return Room{}, err
	}
	return s.store.CreateRoom(ctx, r)
}

// ListRooms devuelve las salas ordenadas por id.
func (s *Service) ListRooms(ctx context.Context) ([]Room, error) {
	return s.store.ListRooms(ctx)
}

// GetRoom devuelve una sala.
func (s *Service) GetRoom(ctx context.Context, id uint) (Room, error) {
	return s.store.GetRoom(ctx, id)
}

// UpdateRoom modifica los campos indicados de una sala.
func (s *Service) UpdateRoom(ctx context.Context, id uint, in RoomChanges) (Room, error) {
	var ch RoomUpdate
	if in.Name != nil {
		v, err := checkName("name", *in.Name)
		if err != nil {
			return Room{}, err
		}
		ch.Name = &v
	}
	if in.URL != nil {
		v, err := checkURL(*in.URL)
		if err != nil {
			return Room{}, err
		}
		ch.URL = &v
	}
	if in.AccessKey != nil {
		v, err := accessKeyHash(*in.AccessKey)
		if err != nil {
			return Room{}, err
		}
		ch.AccessKeyHash = &v
	}
	return s.store.UpdateRoom(ctx, id, ch)
}

// DeleteRoom borra (lógicamente) una sala con sus grupos, las presentaciones de esos grupos y
// sus miembros.
func (s *Service) DeleteRoom(ctx context.Context, id uint) error {
	return s.store.DeleteRoom(ctx, id)
}

// JoinRoom añade un usuario a una sala; si la sala tiene clave de ingreso, hay que darla. Para
// quien ya es miembro no cambia nada (created = false); quien vuelve tras salir reactiva su
// pertenencia.
func (s *Service) JoinRoom(ctx context.Context, roomID uint, in Join) (m RoomMember, created bool, err error) {
	if in.UserID == 0 {
		return RoomMember{}, false, invalid("user_id es obligatorio")
	}
	return s.store.AddRoomMember(ctx, roomID, in.UserID, func(r Room) error {
		if r.HasAccessKey() && bcrypt.CompareHashAndPassword([]byte(r.AccessKeyHash), []byte(in.AccessKey)) != nil {
			return ErrWrongAccessKey
		}
		return nil
	})
}

// RoomMembers devuelve los usuarios miembros de una sala, ordenados por id.
func (s *Service) RoomMembers(ctx context.Context, roomID uint) ([]User, error) {
	return s.store.RoomMembers(ctx, roomID)
}

// LeaveRoom saca a un usuario de una sala y de los grupos de esa sala.
func (s *Service) LeaveRoom(ctx context.Context, roomID, userID uint) error {
	return s.store.RemoveRoomMember(ctx, roomID, userID)
}
