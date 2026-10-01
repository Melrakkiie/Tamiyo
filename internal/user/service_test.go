package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type fakeRepository struct {
	createdUser User
	createErr   error

	findByEmailUser User
	findByEmailErr  error
}

func (f *fakeRepository) Create(ctx context.Context, u User) (User, error) {
	if f.createErr != nil {
		return User{}, f.createErr
	}
	f.createdUser = u
	u.ID = "fake-id"
	return u, nil
}

func (f *fakeRepository) FindByEmail(ctx context.Context, email string) (User, error) {
	if f.findByEmailErr != nil {
		return User{}, f.findByEmailErr
	}
	return f.findByEmailUser, nil
}

func TestService_Register_HashesPasswordBeforeStoring(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret")

	require.NoError(t, err)
	assert.NotEqual(t, "supersecret", repo.createdUser.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.createdUser.PasswordHash), []byte("supersecret")))
}

func TestService_Register_PropagatesEmailAlreadyTakenError(t *testing.T) {
	repo := &fakeRepository{createErr: ErrEmailAlreadyTaken}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret")

	assert.ErrorIs(t, err, ErrEmailAlreadyTaken)
}

func TestService_Authenticate_SucceedsWithCorrectPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("supersecret"), bcrypt.DefaultCost)
	repo := &fakeRepository{findByEmailUser: User{ID: "fake-id", Email: "alice@example.com", PasswordHash: string(hash)}}
	service := NewService(repo)

	result, err := service.Authenticate(context.Background(), "alice@example.com", "supersecret")

	require.NoError(t, err)
	assert.Equal(t, "fake-id", result.ID)
}

func TestService_Authenticate_FailsWithWrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("supersecret"), bcrypt.DefaultCost)
	repo := &fakeRepository{findByEmailUser: User{ID: "fake-id", Email: "alice@example.com", PasswordHash: string(hash)}}
	service := NewService(repo)

	_, err := service.Authenticate(context.Background(), "alice@example.com", "wrongpassword")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestService_Authenticate_FailsWithUnknownEmail(t *testing.T) {
	repo := &fakeRepository{findByEmailErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.Authenticate(context.Background(), "unknown@example.com", "anything")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}
