package domain

import (
	"context"
	"errors"
)

// Errores del dominio. El handler HTTP los traduce a códigos de estado; Store también los usa.
var (
	// ErrInvalid indica datos de entrada inválidos; el mensaje dice cuál y por qué.
	ErrInvalid = errors.New("datos inválidos")

	ErrUserNotFound         = errors.New("usuario no encontrado")
	ErrRoomNotFound         = errors.New("sala no encontrada")
	ErrGroupNotFound        = errors.New("grupo no encontrado")
	ErrPresentationNotFound = errors.New("presentación no encontrada en el grupo")
	ErrNotRoomMember        = errors.New("el usuario no es miembro de la sala")
	ErrNotGroupMember       = errors.New("el usuario no es integrante del grupo")

	ErrEmailTaken             = errors.New("ya existe un usuario con ese correo")
	ErrPresentationAssigned   = errors.New("la presentación ya está asignada")
	ErrRoomMembershipRequired = errors.New("el usuario tiene que ser miembro de la sala para unirse a uno de sus grupos")
	ErrGroupFull              = errors.New("el grupo está completo")
	ErrLimitBelowMembers      = errors.New("el cupo no puede ser menor que los integrantes actuales")
	ErrWrongAccessKey         = errors.New("clave de ingreso incorrecta")

	// ErrPresentationUnknown indica que el servicio presentations no tiene esa presentación.
	ErrPresentationUnknown = errors.New("la presentación no existe en el servicio presentations")
	// ErrCatalogUnavailable indica que no se pudo consultar el servicio presentations.
	ErrCatalogUnavailable = errors.New("no se pudo consultar el servicio presentations")
)

// Store guarda las entidades (lo implementa el paquete store). Recibe datos ya validados y
// devuelve los errores de este paquete: ErrUserNotFound, ErrEmailTaken...
//
// Garantiza, de forma atómica aunque haya peticiones concurrentes:
//   - Cada alta comprueba que su padre existe y no está borrado.
//   - Los borrados son lógicos y en cascada: una sala se lleva sus grupos (con sus presentaciones
//     e integrantes) y sus miembros; un grupo, sus presentaciones e integrantes; un usuario, sus
//     pertenencias a salas y grupos. Quien sale de una sala sale también de sus grupos.
//   - Un grupo nunca supera su cupo: AddGroupMember devuelve ErrGroupFull si está completo, y
//     UpdateGroup, ErrLimitBelowMembers si el cupo nuevo es menor que los integrantes actuales.
//   - Solo los miembros de una sala pueden estar en sus grupos (ErrRoomMembershipRequired).
type Store interface {
	CreateUser(ctx context.Context, u User) (User, error)
	ListUsers(ctx context.Context) ([]User, error)
	GetUser(ctx context.Context, id uint) (User, error)
	UpdateUser(ctx context.Context, id uint, ch UserUpdate) (User, error)
	DeleteUser(ctx context.Context, id uint) error
	UserRooms(ctx context.Context, userID uint) ([]Room, error)

	CreateRoom(ctx context.Context, r Room) (Room, error)
	ListRooms(ctx context.Context) ([]Room, error)
	GetRoom(ctx context.Context, id uint) (Room, error)
	UpdateRoom(ctx context.Context, id uint, ch RoomUpdate) (Room, error)
	DeleteRoom(ctx context.Context, id uint) error
	// AddRoomMember une a un usuario a una sala. admit recibe la sala, bloqueada mientras dura el
	// alta, y decide si el usuario puede entrar. created es false si ya era miembro.
	AddRoomMember(ctx context.Context, roomID, userID uint, admit func(Room) error) (m RoomMember, created bool, err error)
	RoomMembers(ctx context.Context, roomID uint) ([]User, error)
	RemoveRoomMember(ctx context.Context, roomID, userID uint) error

	CreateGroup(ctx context.Context, g Group) (Group, error)
	// RoomGroups devuelve los grupos de una sala en orden de paso: por prioridad y por id.
	RoomGroups(ctx context.Context, roomID uint) ([]Group, error)
	GetGroup(ctx context.Context, id uint) (Group, error)
	UpdateGroup(ctx context.Context, id uint, ch GroupFields) (Group, error)
	DeleteGroup(ctx context.Context, id uint) error
	// AddGroupMember une a un usuario a un grupo. created es false si ya era integrante.
	AddGroupMember(ctx context.Context, groupID, userID uint) (m GroupMember, created bool, err error)
	GroupMembers(ctx context.Context, groupID uint) ([]User, error)
	RemoveGroupMember(ctx context.Context, groupID, userID uint) error
	UserGroups(ctx context.Context, userID uint) ([]Group, error)

	// AddPresentation devuelve ErrPresentationAssigned si la presentación ya está en un grupo.
	AddPresentation(ctx context.Context, p Presentation) (Presentation, error)
	GroupPresentations(ctx context.Context, groupID uint) ([]Presentation, error)
	GetPresentation(ctx context.Context, groupID uint, id string) (Presentation, error)
	SetSlides(ctx context.Context, groupID uint, id string, slides []int32) (Presentation, error)
	RemovePresentation(ctx context.Context, groupID uint, id string) error
}

// UserUpdate son los cambios ya validados de un usuario; los nil no cambian.
type UserUpdate struct {
	FullName     *string
	Email        *string
	PasswordHash *string
	Roles        *[]string
}

// RoomUpdate son los cambios ya validados de una sala; los nil no cambian. Un AccessKeyHash
// vacío quita la clave.
type RoomUpdate struct {
	Name          *string
	URL           *string
	AccessKeyHash *string
}

// Catalog consulta las presentaciones del servicio presentations (lo implementa el paquete
// catalog). SlideCount devuelve ErrPresentationUnknown si el id no existe.
type Catalog interface {
	SlideCount(ctx context.Context, presentationID string) (int, error)
}

// Service implementa los casos de uso: valida y normaliza los datos, guarda los secretos como
// hash bcrypt, comprueba las claves de ingreso y valida las presentaciones contra el servicio
// presentations. Es seguro para uso concurrente.
type Service struct {
	store   Store
	catalog Catalog
}

// NewService crea el servicio.
func NewService(store Store, catalog Catalog) *Service {
	return &Service{store: store, catalog: catalog}
}
