package store

import "context"

type userRepositoryImpl struct {
	st *Store
}

func (s *Store) UserRepository() UserRepository {
	return &userRepositoryImpl{st: s}
}

func (r *userRepositoryImpl) FindByID(ctx context.Context, id string) (*User, error) {
	user, err := r.st.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepositoryImpl) FindByEmail(ctx context.Context, email string) (*User, error) {
	user, err := r.st.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepositoryImpl) Create(ctx context.Context, user *User) error {
	created, err := r.st.CreateUser(ctx, CreateUserRequest{
		Email:    user.Email,
		Password: "changeme",
		Role:     user.Role,
	}, nil)
	if err != nil {
		return err
	}
	*user = created
	return nil
}

func (r *userRepositoryImpl) Update(ctx context.Context, user *User) error {
	return nil
}

func (r *userRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.st.DeleteUser(ctx, id, nil)
}

func (r *userRepositoryImpl) List(ctx context.Context, filter UserFilter) ([]User, error) {
	return r.st.ListUsers(ctx)
}
