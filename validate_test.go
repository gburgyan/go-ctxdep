package ctxdep

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_BasicValidation(t *testing.T) {
	t.Run("successful validation", func(t *testing.T) {
		type User struct {
			Name string
			Age  int
		}

		user := &User{Name: "John", Age: 25}

		validator := func(ctx context.Context, u *User) error {
			if u.Age < 18 {
				return errors.New("user must be 18 or older")
			}
			return nil
		}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			user,
			Validate(validator),
		)

		require.NoError(t, err)
		assert.NotNil(t, ctx)

		// Verify the user is still accessible
		retrievedUser := Get[*User](ctx)
		assert.Equal(t, user, retrievedUser)
	})

	t.Run("failed validation", func(t *testing.T) {
		type User struct {
			Name string
			Age  int
		}

		user := &User{Name: "Jane", Age: 16}

		validator := func(ctx context.Context, u *User) error {
			if u.Age < 18 {
				return errors.New("user must be 18 or older")
			}
			return nil
		}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			user,
			Validate(validator),
		)

		require.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "user must be 18 or older")
	})
}

func TestValidate_MultipleValidators(t *testing.T) {
	type User struct {
		Name  string
		Email string
		Age   int
	}

	user := &User{Name: "John", Email: "john@example.com", Age: 25}

	ageValidator := func(ctx context.Context, u *User) error {
		if u.Age < 18 {
			return errors.New("user must be 18 or older")
		}
		return nil
	}

	emailValidator := func(ctx context.Context, u *User) error {
		if u.Email == "" {
			return errors.New("email is required")
		}
		// Simple email validation
		if len(u.Email) == 0 || u.Email[0] == '@' || u.Email[len(u.Email)-1] == '@' {
			return errors.New("invalid email format")
		}
		atCount := 0
		for _, ch := range u.Email {
			if ch == '@' {
				atCount++
			}
		}
		if atCount != 1 {
			return errors.New("invalid email format")
		}
		return nil
	}

	t.Run("all validators pass", func(t *testing.T) {
		ctx, err := NewDependencyContextWithValidation(context.Background(),
			user,
			Validate(ageValidator),
			Validate(emailValidator),
		)

		require.NoError(t, err)
		assert.NotNil(t, ctx)
	})

	t.Run("first validator fails", func(t *testing.T) {
		youngUser := &User{Name: "Jane", Email: "jane@example.com", Age: 16}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			youngUser,
			Validate(ageValidator),
			Validate(emailValidator),
		)

		require.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "user must be 18 or older")
	})

	t.Run("second validator fails", func(t *testing.T) {
		badEmailUser := &User{Name: "John", Email: "invalid-email", Age: 25}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			badEmailUser,
			Validate(ageValidator),
			Validate(emailValidator),
		)

		require.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "invalid email format")
	})
}

func TestValidate_WithDependencies(t *testing.T) {
	type Database struct {
		Connected bool
	}

	type User struct {
		ID   string
		Name string
	}

	db := &Database{Connected: true}
	user := &User{ID: "123", Name: "John"}

	// Validator that uses multiple dependencies
	validator := func(ctx context.Context, u *User, d *Database) error {
		if !d.Connected {
			return errors.New("database not connected")
		}
		if u.ID == "" {
			return errors.New("user ID is required")
		}
		return nil
	}

	t.Run("validation with dependencies succeeds", func(t *testing.T) {
		ctx, err := NewDependencyContextWithValidation(context.Background(),
			db,
			user,
			Validate(validator),
		)

		require.NoError(t, err)
		assert.NotNil(t, ctx)
	})

	t.Run("validation fails when dependency check fails", func(t *testing.T) {
		disconnectedDB := &Database{Connected: false}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			disconnectedDB,
			user,
			Validate(validator),
		)

		require.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "database not connected")
	})
}

func TestValidate_WithGenerators(t *testing.T) {
	type Config struct {
		MaxUsers int
	}

	type UserCount struct {
		Count int
	}

	config := &Config{MaxUsers: 100}

	// Generator that creates UserCount
	userCountGen := func(ctx context.Context) *UserCount {
		return &UserCount{Count: 50}
	}

	// Validator that uses generated dependency
	validator := func(ctx context.Context, cfg *Config, uc *UserCount) error {
		if uc.Count >= cfg.MaxUsers {
			return errors.New("user limit exceeded")
		}
		return nil
	}

	t.Run("validation with generator succeeds", func(t *testing.T) {
		ctx, err := NewDependencyContextWithValidation(context.Background(),
			config,
			userCountGen,
			Validate(validator),
		)

		require.NoError(t, err)
		assert.NotNil(t, ctx)

		// Verify generator result is available
		uc := Get[*UserCount](ctx)
		assert.Equal(t, 50, uc.Count)
	})

	t.Run("validation with generator fails", func(t *testing.T) {
		// Generator that creates too many users
		tooManyUsersGen := func(ctx context.Context) *UserCount {
			return &UserCount{Count: 150}
		}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			config,
			tooManyUsersGen,
			Validate(validator),
		)

		require.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "user limit exceeded")
	})
}

func TestValidate_InvalidValidatorPanics(t *testing.T) {
	t.Run("non-function panics", func(t *testing.T) {
		assert.Panics(t, func() {
			Validate("not a function")
		})
	})

	t.Run("function without error return panics", func(t *testing.T) {
		assert.Panics(t, func() {
			Validate(func() string { return "test" })
		})
	})

	t.Run("function with multiple returns panics", func(t *testing.T) {
		assert.Panics(t, func() {
			Validate(func() (string, error) { return "test", nil })
		})
	})

	t.Run("function with no parameters panics", func(t *testing.T) {
		assert.Panics(t, func() {
			Validate(func() error { return nil })
		})
	})
}

func TestValidate_BackwardCompatibility(t *testing.T) {
	// Ensure NewDependencyContext still panics on validation failure
	type User struct {
		Age int
	}

	user := &User{Age: 16}

	validator := func(ctx context.Context, u *User) error {
		if u.Age < 18 {
			return errors.New("user must be 18 or older")
		}
		return nil
	}

	// This should panic
	assert.Panics(t, func() {
		NewDependencyContext(context.Background(),
			user,
			Validate(validator),
		)
	})
}

func TestValidate_WithAdapters(t *testing.T) {
	type User struct {
		ID string
	}

	type UserService struct{}

	type UserValidator func(context.Context, string) error

	// Adapter function that will be turned into UserValidator
	validateUserExists := func(ctx context.Context, svc *UserService, userID string) error {
		if userID == "" {
			return errors.New("user ID cannot be empty")
		}
		return nil
	}

	user := &User{ID: "123"}
	svc := &UserService{}

	// Validator that uses the adapted function
	validator := func(ctx context.Context, u *User, validator UserValidator) error {
		return validator(ctx, u.ID)
	}

	t.Run("validation with adapter succeeds", func(t *testing.T) {
		ctx, err := NewDependencyContextWithValidation(context.Background(),
			user,
			svc,
			Adapt[UserValidator](validateUserExists),
			Validate(validator),
		)

		require.NoError(t, err)
		assert.NotNil(t, ctx)
	})

	t.Run("validation with adapter fails", func(t *testing.T) {
		emptyUser := &User{ID: ""}

		ctx, err := NewDependencyContextWithValidation(context.Background(),
			emptyUser,
			svc,
			Adapt[UserValidator](validateUserExists),
			Validate(validator),
		)

		require.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "user ID cannot be empty")
	})
}
