package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"spd.web/services/rooms/internal/domain"
)

func (s *Store) CreateGroup(ctx context.Context, g domain.Group) (domain.Group, error) {
	row := group{RoomID: g.RoomID, Limit: g.Limit, Priority: g.Priority}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := take(tx.Clauses(lockShare), &room{}, domain.ErrRoomNotFound, "id = ?", g.RoomID); err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return domain.Group{}, err
	}
	return row.toDomain(), nil
}

// RoomGroups devuelve los grupos en orden de paso: por prioridad (1 primero) y por id.
func (s *Store) RoomGroups(ctx context.Context, roomID uint) ([]domain.Group, error) {
	db := s.db.WithContext(ctx)
	if err := take(db, &room{}, domain.ErrRoomNotFound, "id = ?", roomID); err != nil {
		return nil, err
	}
	var rows []group
	err := db.Where("room_id = ?", roomID).Order("priority, id").Find(&rows).Error
	return mapList(rows, group.toDomain), err
}

func (s *Store) GetGroup(ctx context.Context, id uint) (domain.Group, error) {
	var row group
	err := take(s.db.WithContext(ctx), &row, domain.ErrGroupNotFound, "id = ?", id)
	return row.toDomain(), err
}

// UpdateGroup no deja el cupo por debajo de los integrantes actuales. El grupo queda bloqueado
// (FOR UPDATE), así que ningún alta concurrente cambia la cuenta mientras tanto.
func (s *Store) UpdateGroup(ctx context.Context, id uint, ch domain.GroupFields) (domain.Group, error) {
	changes := map[string]any{}
	if ch.Limit != nil {
		changes["limit"] = *ch.Limit
	}
	if ch.Priority != nil {
		changes["priority"] = *ch.Priority
	}
	var row group
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := take(tx.Clauses(lockUpdate), &row, domain.ErrGroupNotFound, "id = ?", id); err != nil {
			return err
		}
		if ch.Limit != nil {
			n, err := countMembers(tx, id)
			if err != nil {
				return err
			}
			if int64(*ch.Limit) < n {
				return fmt.Errorf("%w (%d)", domain.ErrLimitBelowMembers, n)
			}
		}
		if len(changes) == 0 {
			return nil
		}
		return tx.Model(&row).Updates(changes).Error
	})
	if err != nil {
		return domain.Group{}, err
	}
	return row.toDomain(), nil
}

// DeleteGroup borra el grupo, sus presentaciones y sus integrantes.
func (s *Store) DeleteGroup(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := deleted(tx.Delete(&group{}, id), domain.ErrGroupNotFound); err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&presentation{}).Error; err != nil {
			return err
		}
		return tx.Where("group_id = ?", id).Delete(&groupMember{}).Error
	})
}

// AddGroupMember bloquea el grupo con FOR UPDATE: las altas en un mismo grupo pasan de a una, así
// dos no pueden ocupar a la vez la última plaza del cupo. La pertenencia del usuario a la sala se
// bloquea con FOR SHARE, para que no salga de ella (ni se borre) mientras entra al grupo.
func (s *Store) AddGroupMember(ctx context.Context, groupID, userID uint) (domain.GroupMember, bool, error) {
	var m groupMember
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g group
		if err := take(tx.Clauses(lockUpdate), &g, domain.ErrGroupNotFound, "id = ?", groupID); err != nil {
			return err
		}
		if err := take(tx.Clauses(lockShare), &user{}, domain.ErrUserNotFound, "id = ?", userID); err != nil {
			return err
		}
		if err := take(tx.Clauses(lockShare), &roomMember{}, domain.ErrRoomMembershipRequired,
			"user_id = ? AND room_id = ?", userID, g.RoomID); err != nil {
			return err
		}

		err := tx.Where("user_id = ? AND group_id = ?", userID, groupID).Take(&m).Error
		if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
			return err // ya era integrante (created = false), o falló la consulta
		}
		n, err := countMembers(tx, groupID)
		if err != nil {
			return err
		}
		if n >= int64(g.Limit) {
			return fmt.Errorf("%w (cupo %d)", domain.ErrGroupFull, g.Limit)
		}

		m = groupMember{UserID: userID, GroupID: groupID}
		res := tx.Clauses(reactivate("group_members", []string{"user_id", "group_id"}, "created_at")).Create(&m)
		created = res.Error == nil
		return res.Error
	})
	if err != nil {
		return domain.GroupMember{}, false, err
	}
	return m.toDomain(), created, nil
}

func (s *Store) GroupMembers(ctx context.Context, groupID uint) ([]domain.User, error) {
	db := s.db.WithContext(ctx)
	if err := take(db, &group{}, domain.ErrGroupNotFound, "id = ?", groupID); err != nil {
		return nil, err
	}
	var rows []user
	err := db.Select("users.*").
		Joins("JOIN group_members ON group_members.user_id = users.id AND group_members.deleted_at IS NULL").
		Where("group_members.group_id = ?", groupID).
		Order("users.id").Find(&rows).Error
	return mapList(rows, user.toDomain), err
}

func (s *Store) RemoveGroupMember(ctx context.Context, groupID, userID uint) error {
	res := s.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&groupMember{})
	return deleted(res, domain.ErrNotGroupMember)
}

// countMembers cuenta los integrantes (no borrados) de un grupo.
func countMembers(tx *gorm.DB, groupID uint) (int64, error) {
	var n int64
	err := tx.Model(&groupMember{}).Where("group_id = ?", groupID).Count(&n).Error
	return n, err
}

func (s *Store) AddPresentation(ctx context.Context, p domain.Presentation) (domain.Presentation, error) {
	row := presentation{ID: p.ID, GroupID: p.GroupID, Slides: p.Slides}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := take(tx.Clauses(lockShare), &group{}, domain.ErrGroupNotFound, "id = ?", p.GroupID); err != nil {
			return err
		}
		res := tx.Clauses(reactivate("presentations", []string{"id"}, "group_id", "slides", "created_at", "updated_at")).Create(&row)
		if res.Error != nil || res.RowsAffected == 1 {
			return res.Error
		}
		var current presentation // está activa en algún grupo
		if err := take(tx, &current, domain.ErrPresentationNotFound, "id = ?", p.ID); err != nil {
			return err
		}
		return fmt.Errorf("%w al grupo %d", domain.ErrPresentationAssigned, current.GroupID)
	})
	if err != nil {
		return domain.Presentation{}, err
	}
	return row.toDomain(), nil
}

// GroupPresentations devuelve las presentaciones del grupo en el orden en que se asignaron.
func (s *Store) GroupPresentations(ctx context.Context, groupID uint) ([]domain.Presentation, error) {
	db := s.db.WithContext(ctx)
	if err := take(db, &group{}, domain.ErrGroupNotFound, "id = ?", groupID); err != nil {
		return nil, err
	}
	var rows []presentation
	err := db.Where("group_id = ?", groupID).Order("created_at, id").Find(&rows).Error
	return mapList(rows, presentation.toDomain), err
}

func (s *Store) GetPresentation(ctx context.Context, groupID uint, id string) (domain.Presentation, error) {
	var row presentation
	err := take(s.db.WithContext(ctx), &row, domain.ErrPresentationNotFound, "id = ? AND group_id = ?", id, groupID)
	return row.toDomain(), err
}

func (s *Store) SetSlides(ctx context.Context, groupID uint, id string, slides []int32) (domain.Presentation, error) {
	var row presentation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := take(tx.Clauses(lockUpdate), &row, domain.ErrPresentationNotFound, "id = ? AND group_id = ?", id, groupID); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"slides": Array[int32](slides)}).Error
	})
	if err != nil {
		return domain.Presentation{}, err
	}
	return row.toDomain(), nil
}

func (s *Store) RemovePresentation(ctx context.Context, groupID uint, id string) error {
	res := s.db.WithContext(ctx).Where("id = ? AND group_id = ?", id, groupID).Delete(&presentation{})
	return deleted(res, domain.ErrPresentationNotFound)
}
