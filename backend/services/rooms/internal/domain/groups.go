package domain

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"spd.web/internal/platform"
)

// GroupFields son los campos de un grupo: Limit es su cupo (máximo de integrantes) y Priority, su
// prioridad de paso al presentar (1 presenta primero). En el alta ambos son obligatorios; al
// modificar, los que llegan nil no cambian.
type GroupFields struct {
	Limit    *int `json:"limit"`
	Priority *int `json:"priority"`
}

// GroupJoin son los datos para unirse a un grupo.
type GroupJoin struct {
	UserID uint `json:"user_id"`
}

// NewPresentation asigna una presentación del servicio presentations a un grupo. Slides nil:
// todas sus diapositivas, en orden.
type NewPresentation struct {
	PresentationID string   `json:"presentation_id"`
	Slides         *[]int32 `json:"slides"`
}

// PresentationChanges cambia las diapositivas de una presentación asignada.
type PresentationChanges struct {
	Slides *[]int32 `json:"slides"`
}

// CreateGroup crea un grupo en una sala.
func (s *Service) CreateGroup(ctx context.Context, roomID uint, in GroupFields) (Group, error) {
	switch {
	case in.Limit == nil:
		return Group{}, invalid("limit es obligatorio")
	case in.Priority == nil:
		return Group{}, invalid("priority es obligatorio")
	}
	if err := checkGroup(in); err != nil {
		return Group{}, err
	}
	return s.store.CreateGroup(ctx, Group{RoomID: roomID, Limit: *in.Limit, Priority: *in.Priority})
}

// RoomGroups devuelve los grupos de una sala en el orden en que pasan a presentar.
func (s *Service) RoomGroups(ctx context.Context, roomID uint) ([]Group, error) {
	return s.store.RoomGroups(ctx, roomID)
}

// GetGroup devuelve un grupo.
func (s *Service) GetGroup(ctx context.Context, id uint) (Group, error) {
	return s.store.GetGroup(ctx, id)
}

// UpdateGroup modifica los campos indicados de un grupo. El cupo no puede quedar por debajo de
// los integrantes actuales (ErrLimitBelowMembers).
func (s *Service) UpdateGroup(ctx context.Context, id uint, in GroupFields) (Group, error) {
	if err := checkGroup(in); err != nil {
		return Group{}, err
	}
	return s.store.UpdateGroup(ctx, id, in)
}

// DeleteGroup borra (lógicamente) un grupo, sus presentaciones y sus integrantes.
func (s *Service) DeleteGroup(ctx context.Context, id uint) error {
	return s.store.DeleteGroup(ctx, id)
}

// JoinGroup une a un usuario a un grupo (DetalleGrupo). Tiene que ser miembro de la sala del
// grupo (ErrRoomMembershipRequired) y quedar sitio en el cupo (ErrGroupFull). Para quien ya es
// integrante no cambia nada (created = false); quien vuelve tras salir reactiva su pertenencia.
func (s *Service) JoinGroup(ctx context.Context, groupID uint, in GroupJoin) (m GroupMember, created bool, err error) {
	if in.UserID == 0 {
		return GroupMember{}, false, invalid("user_id es obligatorio")
	}
	return s.store.AddGroupMember(ctx, groupID, in.UserID)
}

// GroupMembers devuelve los usuarios integrantes de un grupo, ordenados por id.
func (s *Service) GroupMembers(ctx context.Context, groupID uint) ([]User, error) {
	return s.store.GroupMembers(ctx, groupID)
}

// LeaveGroup saca a un usuario de un grupo.
func (s *Service) LeaveGroup(ctx context.Context, groupID, userID uint) error {
	return s.store.RemoveGroupMember(ctx, groupID, userID)
}

// AddPresentation asigna una presentación a un grupo. Cada presentación pertenece a un único
// grupo: si ya está asignada, devuelve ErrPresentationAssigned. Una que se quitó antes se
// reactiva con los datos nuevos.
func (s *Service) AddPresentation(ctx context.Context, groupID uint, in NewPresentation) (Presentation, error) {
	id, ok := platform.CanonicalUUID(in.PresentationID)
	if !ok {
		return Presentation{}, invalid("presentation_id debe ser un UUID")
	}
	// Antes de consultar al servicio presentations: un grupo inexistente es un 404, no un 422.
	if _, err := s.store.GetGroup(ctx, groupID); err != nil {
		return Presentation{}, err
	}
	slides, err := s.checkSlides(ctx, id, in.Slides)
	if err != nil {
		return Presentation{}, err
	}
	return s.store.AddPresentation(ctx, Presentation{ID: id, GroupID: groupID, Slides: slides})
}

// GroupPresentations devuelve las presentaciones de un grupo en el orden en que se asignaron.
func (s *Service) GroupPresentations(ctx context.Context, groupID uint) ([]Presentation, error) {
	return s.store.GroupPresentations(ctx, groupID)
}

// GetPresentation devuelve una presentación de un grupo.
func (s *Service) GetPresentation(ctx context.Context, groupID uint, presentationID string) (Presentation, error) {
	id, ok := platform.CanonicalUUID(presentationID)
	if !ok {
		return Presentation{}, ErrPresentationNotFound
	}
	return s.store.GetPresentation(ctx, groupID, id)
}

// UpdatePresentation cambia las diapositivas de una presentación de un grupo.
func (s *Service) UpdatePresentation(ctx context.Context, groupID uint, presentationID string, in PresentationChanges) (Presentation, error) {
	p, err := s.GetPresentation(ctx, groupID, presentationID)
	if err != nil || in.Slides == nil {
		return p, err
	}
	slides, err := s.checkSlides(ctx, p.ID, in.Slides)
	if err != nil {
		return Presentation{}, err
	}
	return s.store.SetSlides(ctx, groupID, p.ID, slides)
}

// RemovePresentation quita (borrado lógico) una presentación de un grupo.
func (s *Service) RemovePresentation(ctx context.Context, groupID uint, presentationID string) error {
	id, ok := platform.CanonicalUUID(presentationID)
	if !ok {
		return ErrPresentationNotFound
	}
	return s.store.RemovePresentation(ctx, groupID, id)
}

// checkSlides valida las diapositivas contra el número real de la presentación, que se consulta
// al servicio presentations: cada una debe existir y no repetirse. Sin lista (nil) se asignan
// todas, de la 1 a la última.
func (s *Service) checkSlides(ctx context.Context, presentationID string, slides *[]int32) ([]int32, error) {
	count, err := s.catalog.SlideCount(ctx, presentationID)
	switch {
	case errors.Is(err, ErrPresentationUnknown):
		return nil, err
	case err != nil:
		return nil, fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	if slides == nil {
		all := make([]int32, count)
		for i := range all {
			all[i] = int32(i + 1)
		}
		return all, nil
	}
	seen := make(map[int32]bool, len(*slides))
	for _, n := range *slides {
		switch {
		case n < 1 || int(n) > count:
			return nil, invalid("la diapositiva %d no existe: la presentación tiene %d", n, count)
		case seen[n]:
			return nil, invalid("la diapositiva %d está repetida", n)
		}
		seen[n] = true
	}
	return slices.Clone(*slides), nil
}
