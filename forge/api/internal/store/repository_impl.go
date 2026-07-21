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
	_, err := r.st.db.Exec(ctx, `
		UPDATE users
		SET email = $1,
		    cpu_limit = $2, memory_mb_limit = $3, disk_mb_limit = $4,
		    backup_limit = $5, database_limit = $6, allocation_limit = $7,
		    subuser_limit = $8, schedule_limit = $9, server_limit = $10,
		    updated_at = now()
		WHERE id = $11
	`, user.Email,
		user.CPULimit, user.MemoryMBLimit, user.DiskMBLimit,
		user.BackupLimit, user.DatabaseLimit, user.AllocationLimit,
		user.SubuserLimit, user.ScheduleLimit, user.ServerLimit,
		user.ID)
	return err
}

func (r *userRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.st.DeleteUser(ctx, id, nil)
}

func (r *userRepositoryImpl) List(ctx context.Context, filter UserFilter) ([]User, error) {
	return r.st.ListUsers(ctx)
}
