package store

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"spd.web/services/rooms/internal/domain"
)

func (s *Store) CreateUser(ctx context.Context, u domain.User) (domain.User, error) {
	row := user{FullName: u.FullName, Email: u.Email, PasswordHash: u.PasswordHash, Roles: u.Roles}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return domain.User{}, emailError(err)
	}
	return row.toDomain(), nil
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	var rows []user
	err := s.db.WithContext(ctx).Order("id").Find(&rows).Error
	return mapList(rows, user.toDomain), err
}

func (s *Store) GetUser(ctx context.Context, id uint) (domain.User, error) {
	var row user
	err := take(s.db.WithContext(ctx), &row, domain.ErrUserNotFound, "id = ?", id)
	return row.toDomain(), err
}

func (s *Store) UpdateUser(ctx context.Context, id uint, ch domain.UserUpdate) (domain.User, error) {
	changes := map[string]any{}
	if ch.FullName != nil {
		changes["full_name"] = *ch.FullName
	}
	if ch.Email != nil {
		changes["email"] = *ch.Email
	}
	if ch.PasswordHash != nil {
		changes["password_hash"] = *ch.PasswordHash
	}
	if ch.Roles != nil {
		changes["roles"] = Array[string](*ch.Roles)
	}
	var row user
	if err := s.update(ctx, &row, id, domain.ErrUserNotFound, changes); err != nil {
		return domain.User{}, emailError(err)
	}
	return row.toDomain(), nil
}

// DeleteUser borra el usuario y sus pertenencias a salas y grupos.
func (s *Store) DeleteUser(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := deleted(tx.Delete(&user{}, id), domain.ErrUserNotFound); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&roomMember{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", id).Delete(&groupMember{}).Error
	})
}

func (s *Store) UserRooms(ctx context.Context, userID uint) ([]domain.Room, error) {
	db := s.db.WithContext(ctx)
	if err := take(db, &user{}, domain.ErrUserNotFound, "id = ?", userID); err != nil {
		return nil, err
	}
	var rows []room
	err := db.Select("rooms.*").
		Joins("JOIN room_members ON room_members.room_id = rooms.id AND room_members.deleted_at IS NULL").
		Where("room_members.user_id = ?", userID).
		Order("rooms.id").Find(&rows).Error
	return mapList(rows, room.toDomain), err
}

// UserGroups devuelve los grupos del usuario, por sala y en orden de paso.
func (s *Store) UserGroups(ctx context.Context, userID uint) ([]domain.Group, error) {
	db := s.db.WithContext(ctx)
	if err := take(db, &user{}, domain.ErrUserNotFound, "id = ?", userID); err != nil {
		return nil, err
	}
	var rows []group
	err := db.Select("groups.*").
		Joins("JOIN group_members ON group_members.group_id = groups.id AND group_members.deleted_at IS NULL").
		Where("group_members.user_id = ?", userID).
		Order("groups.room_id, groups.priority, groups.id").Find(&rows).Error
	return mapList(rows, group.toDomain), err
}

// emailError traduce la violación del índice único de users.email, el único que puede fallar al
// escribir un usuario.
func emailError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrEmailTaken
	}
	return err
}
