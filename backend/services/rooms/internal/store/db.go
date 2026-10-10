package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open conecta con PostgreSQL y limita el pool a maxConns conexiones. Con TranslateError, GORM
// devuelve gorm.ErrDuplicatedKey o gorm.ErrForeignKeyViolated en lugar del error del driver.
func Open(dsn string, maxConns int, log *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		// PostgreSQL guarda microsegundos: así la respuesta de un alta muestra las mismas fechas
		// que las lecturas posteriores.
		NowFunc: func() time.Time { return time.Now().Truncate(time.Microsecond) },
		Logger: quietLogger{logger.NewSlogLogger(log, logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true, // el log nunca muestra valores (hashes, correos)
		})},
	})
	if err != nil {
		return nil, fmt.Errorf("conectar a la base de datos: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(maxConns)
	sqlDB.SetMaxIdleConns(maxConns)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}

// Migrate crea o actualiza las tablas, los índices y las claves foráneas (AutoMigrate de GORM).
// Solo añade: nunca borra ni renombra columnas.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&user{}, &room{}, &roomMember{}, &group{}, &groupMember{}, &presentation{}); err != nil {
		return fmt.Errorf("migrar la base de datos: %w", err)
	}
	return nil
}

// Close cierra el pool de conexiones.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// quietLogger no registra como error de SQL una clave única repetida: es un error del cliente
// (p. ej. un correo que ya existe) que la API responde con 409.
type quietLogger struct{ logger.Interface }

func (l quietLogger) LogMode(level logger.LogLevel) logger.Interface {
	return quietLogger{l.Interface.LogMode(level)}
}

func (l quietLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		err = nil
	}
	l.Interface.Trace(ctx, begin, fc, err)
}
