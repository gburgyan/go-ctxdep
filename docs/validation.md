# Validation

The library supports running validation functions during context creation. This ensures all dependencies meet certain criteria before application logic proceeds.

## Basic Validation

Validators are functions that return an error. They can take any dependencies from the context as parameters:

```go
func validateUser(ctx context.Context, user *User) error {
    if user.Age < 18 {
        return errors.New("user must be 18 or older")
    }
    return nil
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    &User{Name: "John", Age: 25},
    ctxdep.Validate(validateUser),
)
if err != nil {
    log.Fatal(err)
}

// If we get here, validation passed
user := ctxdep.Get[*User](ctx)
```

## NewDependencyContextWithValidation

Unlike `NewDependencyContext` which panics on validation errors, `NewDependencyContextWithValidation` returns an error:

```go
// Returns error instead of panicking
ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    &User{Age: 16},
    ctxdep.Validate(validateAge),
)
if err != nil {
    // Handle validation failure gracefully
    return fmt.Errorf("validation failed: %w", err)
}
```

**Note:** The original `NewDependencyContext` still panics on validation errors for backward compatibility.

## Multiple Validators

You can register multiple validators:

```go
func validateAge(ctx context.Context, user *User) error {
    if user.Age < 18 {
        return errors.New("user must be 18 or older")
    }
    return nil
}

func validateEmail(ctx context.Context, user *User) error {
    if !strings.Contains(user.Email, "@") {
        return errors.New("invalid email format")
    }
    return nil
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    user,
    ctxdep.Validate(validateAge),
    ctxdep.Validate(validateEmail),
)
```

Validators run in order. The first error stops validation.

## Validators with Dependencies

Validators can use multiple dependencies from the context:

```go
func validateOrderLimit(ctx context.Context, user *User, db *Database, order *Order) error {
    count, err := db.GetUserOrderCount(ctx, user.ID)
    if err != nil {
        return err
    }

    if count >= 100 {
        return errors.New("user has reached order limit")
    }

    if order.Total > user.CreditLimit {
        return errors.New("order exceeds credit limit")
    }

    return nil
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    user,
    db,
    order,
    ctxdep.Validate(validateOrderLimit),
)
```

## Validation with Adapters

Validators work seamlessly with adapted functions:

```go
type OrderValidator func(ctx context.Context, orderID string) error

func validateOrderExists(ctx context.Context, db *Database, orderID string) error {
    exists, err := db.OrderExists(ctx, orderID)
    if err != nil {
        return err
    }
    if !exists {
        return errors.New("order not found")
    }
    return nil
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    db,
    orderID,
    ctxdep.Adapt[OrderValidator](validateOrderExists),
    ctxdep.Validate(func(ctx context.Context, validator OrderValidator, id string) error {
        return validator(ctx, id)
    }),
)
```

## Validation Timing

1. **After dependency setup**: All dependencies are initialized first
2. **After adapters**: Adapters are created before validators run
3. **Before immediate generators**: Validators run before `Immediate()` generators start

## Validator Lifecycle

Validators are **not** stored as dependencies. After validation completes:
- Validators are removed from the context
- You cannot retrieve validators with `Get()`

## Use Cases

### Request Validation in API Handlers

```go
func CreateOrderHandler(w http.ResponseWriter, r *http.Request) {
    var orderRequest CreateOrderRequest
    json.NewDecoder(r.Body).Decode(&orderRequest)

    ctx, err := ctxdep.NewDependencyContextWithValidation(r.Context(),
        &orderRequest,
        ctxdep.Get[*User](r.Context()),
        ctxdep.Get[*Database](r.Context()),
        ctxdep.Validate(validateOrderRequest),
        ctxdep.Validate(validateUserCanOrder),
        ctxdep.Validate(validateInventory),
    )

    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Process the valid order...
}
```

### Business Rule Enforcement

```go
func validateBusinessRules(ctx context.Context, order *Order, customer *Customer) error {
    if customer.AccountStatus == "suspended" {
        return errors.New("account is suspended")
    }
    if order.Total > customer.CreditLimit {
        return errors.New("exceeds credit limit")
    }
    return nil
}
```

### Configuration Validation

```go
func validateConfig(ctx context.Context, cfg *Config) error {
    if cfg.DatabaseURL == "" {
        return errors.New("database URL is required")
    }
    if cfg.MaxConnections < 1 {
        return errors.New("max connections must be at least 1")
    }
    return nil
}
```

---

## See Also

- [Testing](testing.md) - Testing validation logic
- [Core Concepts](core-concepts.md) - Fail-fast philosophy
