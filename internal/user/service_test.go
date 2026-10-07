package user

import (
	"context"
	"strings"
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

	findByIDUser User
	findByIDErr  error

	updatePasswordID   string
	updatePasswordHash string
	updatePasswordErr  error

	updateEmailID    string
	updateEmailValue string
	updateEmailErr   error

	updateDisplayNameID    string
	updateDisplayNameValue *string
	updateDisplayNameErr   error

	updateAvatarID    string
	updateAvatarValue *string
}

func (f *fakeRepository) UpdateAvatar(ctx context.Context, id string, avatarScryfallID *string) (User, error) {
	f.updateAvatarID = id
	f.updateAvatarValue = avatarScryfallID
	return User{ID: id, Email: "alice@example.com", AvatarScryfallID: avatarScryfallID}, nil
}

func (f *fakeRepository) UpdateDisplayName(ctx context.Context, id string, displayName *string) (User, error) {
	if f.updateDisplayNameErr != nil {
		return User{}, f.updateDisplayNameErr
	}
	f.updateDisplayNameID = id
	f.updateDisplayNameValue = displayName
	return User{ID: id, Email: "alice@example.com", DisplayName: displayName}, nil
}

func (f *fakeRepository) UpdateEmail(ctx context.Context, id string, email string) error {
	if f.updateEmailErr != nil {
		return f.updateEmailErr
	}
	f.updateEmailID = id
	f.updateEmailValue = email
	return nil
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

func (f *fakeRepository) FindByID(ctx context.Context, id string) (User, error) {
	if f.findByIDErr != nil {
		return User{}, f.findByIDErr
	}
	return f.findByIDUser, nil
}

func (f *fakeRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	if f.updatePasswordErr != nil {
		return f.updatePasswordErr
	}
	f.updatePasswordID = id
	f.updatePasswordHash = passwordHash
	return nil
}

func TestService_Register_HashesPasswordBeforeStoring(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret", nil)

	require.NoError(t, err)
	assert.NotEqual(t, "supersecret", repo.createdUser.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.createdUser.PasswordHash), []byte("supersecret")))
}

func TestService_Register_PropagatesEmailAlreadyTakenError(t *testing.T) {
	repo := &fakeRepository{createErr: ErrEmailAlreadyTaken}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret", nil)

	assert.ErrorIs(t, err, ErrEmailAlreadyTaken)
}

func TestService_Register_ReturnsErrorWhenPasswordTooLongToHash(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	tooLong := strings.Repeat("a", 73) // bcrypt rejects passwords over 72 bytes

	_, err := service.Register(context.Background(), "alice@example.com", tooLong, nil)

	require.Error(t, err)
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

func TestService_ChangePassword_UpdatesHashWhenCurrentPasswordIsCorrect(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpassword"), bcrypt.DefaultCost)
	repo := &fakeRepository{findByIDUser: User{ID: "fake-id", PasswordHash: string(hash)}}
	service := NewService(repo)

	err := service.ChangePassword(context.Background(), "fake-id", "oldpassword", "newpassword")

	require.NoError(t, err)
	assert.Equal(t, "fake-id", repo.updatePasswordID)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.updatePasswordHash), []byte("newpassword")))
}

func TestService_ChangePassword_FailsWithIncorrectCurrentPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpassword"), bcrypt.DefaultCost)
	repo := &fakeRepository{findByIDUser: User{ID: "fake-id", PasswordHash: string(hash)}}
	service := NewService(repo)

	err := service.ChangePassword(context.Background(), "fake-id", "wrongpassword", "newpassword")

	assert.ErrorIs(t, err, ErrIncorrectPassword)
	assert.Empty(t, repo.updatePasswordHash, "password must not be updated when the current one is wrong")
}

func TestService_ChangePassword_PropagatesFindByIDError(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	err := service.ChangePassword(context.Background(), "unknown-id", "whatever", "newpassword")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_ChangePassword_PropagatesUpdatePasswordError(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpassword"), bcrypt.DefaultCost)
	repo := &fakeRepository{
		findByIDUser:      User{ID: "fake-id", PasswordHash: string(hash)},
		updatePasswordErr: ErrNotFound,
	}
	service := NewService(repo)

	err := service.ChangePassword(context.Background(), "fake-id", "oldpassword", "newpassword")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_FindIDByEmail_ReturnsIDOnSuccess(t *testing.T) {
	repo := &fakeRepository{findByEmailUser: User{ID: "fake-id", Email: "alice@example.com"}}
	service := NewService(repo)

	id, err := service.FindIDByEmail(context.Background(), "alice@example.com")

	require.NoError(t, err)
	assert.Equal(t, "fake-id", id)
}

func TestService_FindIDByEmail_PropagatesErrNotFound(t *testing.T) {
	repo := &fakeRepository{findByEmailErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.FindIDByEmail(context.Background(), "unknown@example.com")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_SetPassword_UpdatesHashWithoutCheckingCurrentPassword(t *testing.T) {
	repo := &fakeRepository{findByIDUser: User{ID: "fake-id", PasswordHash: "irrelevant-old-hash"}}
	service := NewService(repo)

	err := service.SetPassword(context.Background(), "fake-id", "brandnewpassword")

	require.NoError(t, err)
	assert.Equal(t, "fake-id", repo.updatePasswordID)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.updatePasswordHash), []byte("brandnewpassword")))
}

func TestService_SetPassword_ReturnsErrorWhenPasswordTooLongToHash(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	tooLong := strings.Repeat("a", 73) // bcrypt rejects passwords over 72 bytes

	err := service.SetPassword(context.Background(), "fake-id", tooLong)

	require.Error(t, err)
}

func TestService_SetPassword_PropagatesUpdatePasswordError(t *testing.T) {
	repo := &fakeRepository{updatePasswordErr: ErrNotFound}
	service := NewService(repo)

	err := service.SetPassword(context.Background(), "unknown-id", "brandnewpassword")

	assert.ErrorIs(t, err, ErrNotFound)
}

func hashedUser(t *testing.T, email, password string) User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return User{ID: "user-1", Email: email, PasswordHash: string(hash)}
}

func TestService_CheckPassword_ReturnsCurrentEmailWhenPasswordIsCorrect(t *testing.T) {
	repo := &fakeRepository{findByIDUser: hashedUser(t, "alice@example.com", "supersecret")}
	service := NewService(repo)

	email, err := service.CheckPassword(context.Background(), "user-1", "supersecret")

	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", email)
}

func TestService_CheckPassword_FailsWithIncorrectPassword(t *testing.T) {
	repo := &fakeRepository{findByIDUser: hashedUser(t, "alice@example.com", "supersecret")}
	service := NewService(repo)

	_, err := service.CheckPassword(context.Background(), "user-1", "wrong")

	assert.ErrorIs(t, err, ErrIncorrectPassword)
}

func TestService_EmailTaken_IsFalseWhenNoUserHasIt(t *testing.T) {
	service := NewService(&fakeRepository{findByEmailErr: ErrNotFound})

	taken, err := service.EmailTaken(context.Background(), "new@example.com")

	require.NoError(t, err)
	assert.False(t, taken)
}

func TestService_EmailTaken_IsTrueWhenAUserHasIt(t *testing.T) {
	service := NewService(&fakeRepository{findByEmailUser: User{ID: "user-2", Email: "new@example.com"}})

	taken, err := service.EmailTaken(context.Background(), "new@example.com")

	require.NoError(t, err)
	assert.True(t, taken)
}

func TestService_EmailTaken_PropagatesOtherErrors(t *testing.T) {
	service := NewService(&fakeRepository{findByEmailErr: assert.AnError})

	_, err := service.EmailTaken(context.Background(), "new@example.com")

	assert.ErrorIs(t, err, assert.AnError)
}

func TestService_ChangeEmail_UpdatesEmailAndReturnsTheOldOne(t *testing.T) {
	repo := &fakeRepository{findByIDUser: User{ID: "user-1", Email: "alice@example.com"}}
	service := NewService(repo)

	old, err := service.ChangeEmail(context.Background(), "user-1", "new@example.com")

	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", old)
	assert.Equal(t, "user-1", repo.updateEmailID)
	assert.Equal(t, "new@example.com", repo.updateEmailValue)
}

func TestService_ChangeEmail_PropagatesEmailAlreadyTaken(t *testing.T) {
	repo := &fakeRepository{findByIDUser: User{ID: "user-1", Email: "alice@example.com"}, updateEmailErr: ErrEmailAlreadyTaken}
	service := NewService(repo)

	_, err := service.ChangeEmail(context.Background(), "user-1", "new@example.com")

	assert.ErrorIs(t, err, ErrEmailAlreadyTaken)
}

func strPtr(s string) *string {
	return &s
}

func TestService_SetDisplayName_StoresTheTrimmedName(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	u, err := service.SetDisplayName(context.Background(), "user-1", strPtr("  Tamiyo  "))

	require.NoError(t, err)
	assert.Equal(t, "user-1", repo.updateDisplayNameID)
	require.NotNil(t, repo.updateDisplayNameValue)
	assert.Equal(t, "Tamiyo", *repo.updateDisplayNameValue)
	assert.Equal(t, "Tamiyo", *u.DisplayName)
}

func TestService_SetDisplayName_ClearsTheNameWhenNilOrBlank(t *testing.T) {
	for _, name := range []*string{nil, strPtr(""), strPtr("   ")} {
		repo := &fakeRepository{}
		service := NewService(repo)

		_, err := service.SetDisplayName(context.Background(), "user-1", name)

		require.NoError(t, err)
		assert.Equal(t, "user-1", repo.updateDisplayNameID)
		assert.Nil(t, repo.updateDisplayNameValue)
	}
}

func TestService_SetDisplayName_AcceptsThirtyTwoCharactersCountedAsLetters(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.SetDisplayName(context.Background(), "user-1", strPtr(strings.Repeat("é", 32)))

	require.NoError(t, err)
}

func TestService_SetDisplayName_RejectsALongerName(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.SetDisplayName(context.Background(), "user-1", strPtr(strings.Repeat("a", 33)))

	assert.ErrorIs(t, err, ErrInvalidDisplayName)
	assert.Empty(t, repo.updateDisplayNameID)
}

func TestService_Register_StoresTheTrimmedDisplayName(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret", strPtr("  Tamiyo "))

	require.NoError(t, err)
	require.NotNil(t, repo.createdUser.DisplayName)
	assert.Equal(t, "Tamiyo", *repo.createdUser.DisplayName)
}

func TestService_Register_LeavesABlankDisplayNameUnset(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret", strPtr("   "))

	require.NoError(t, err)
	assert.Nil(t, repo.createdUser.DisplayName)
}

func TestService_Register_RejectsATooLongDisplayNameBeforeCreatingTheAccount(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.Register(context.Background(), "alice@example.com", "supersecret", strPtr(strings.Repeat("a", 33)))

	assert.ErrorIs(t, err, ErrInvalidDisplayName)
	assert.Empty(t, repo.createdUser.Email)
}

func TestService_SetAvatar_StoresTheLowercasedScryfallID(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.SetAvatar(context.Background(), "user-1", strPtr(" 0000579F-7B35-4ED3-B44C-DB2A538066FE "))

	require.NoError(t, err)
	assert.Equal(t, "user-1", repo.updateAvatarID)
	require.NotNil(t, repo.updateAvatarValue)
	assert.Equal(t, "0000579f-7b35-4ed3-b44c-db2a538066fe", *repo.updateAvatarValue)
}

func TestService_SetAvatar_ClearsTheAvatarWhenNilOrBlank(t *testing.T) {
	for _, id := range []*string{nil, strPtr(""), strPtr("  ")} {
		repo := &fakeRepository{}
		service := NewService(repo)

		_, err := service.SetAvatar(context.Background(), "user-1", id)

		require.NoError(t, err)
		assert.Equal(t, "user-1", repo.updateAvatarID)
		assert.Nil(t, repo.updateAvatarValue)
	}
}

func TestService_SetAvatar_RejectsSomethingThatIsNotAScryfallID(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.SetAvatar(context.Background(), "user-1", strPtr("https://cards.scryfall.io/art_crop/front/0/0/0000579f.jpg"))

	assert.ErrorIs(t, err, ErrInvalidAvatar)
	assert.Empty(t, repo.updateAvatarID)
}
