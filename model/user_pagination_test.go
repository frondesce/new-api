package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUsersForPaginationTest(t *testing.T, total int) {
	t.Helper()
	for id := 1; id <= total; id++ {
		user := &User{
			Id:          id,
			Username:    fmt.Sprintf("user%02d", id),
			Password:    "password123",
			DisplayName: fmt.Sprintf("User %02d", id),
			Email:       fmt.Sprintf("user%02d@example.com", id),
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			Group:       "default",
			AffCode:     fmt.Sprintf("aff%02d", id),
		}
		require.NoError(t, DB.Create(user).Error)
	}
}

func collectUserIDs(users []*User) []int {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	return ids
}

func TestGetAllUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	pageOne, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, collectUserIDs(pageOne))

	pageTwo, total, err := GetAllUsers(&common.PageInfo{Page: 2, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(pageTwo))

	pageThree, total, err := GetAllUsers(&common.PageInfo{Page: 3, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{41, 42}, collectUserIDs(pageThree))
}

func TestSearchUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	users, total, err := SearchUsers("user", "", nil, nil, 20, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(users))
}

func TestUserListsHideDeletedUsersByDefault(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 3)
	require.NoError(t, DB.Delete(&User{}, 2).Error)

	users, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, []int{1, 3}, collectUserIDs(users))

	users, total, err = SearchUsers("user", "", nil, nil, 0, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, []int{1, 3}, collectUserIDs(users))

	deletedStatus := -1
	users, total, err = SearchUsers("user", "", nil, &deletedStatus, 0, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []int{2}, collectUserIDs(users))
}

func TestUserListsPreservePaginationAndDeletedFilterAcrossDatabases(t *testing.T) {
	for _, engine := range []struct {
		name string
		env  string
	}{
		{name: "mysql", env: "TEST_MYSQL_DSN"},
		{name: "postgres", env: "TEST_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			t.Setenv("USER_LIST_TEST_DSN", dsn)
			db, databaseType, err := chooseDB("USER_LIST_TEST_DSN", false)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			previousDB, previousDatabaseType := DB, common.MainDatabaseType()
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousDatabaseType)
				initCol()
				_ = sqlDB.Close()
			})
			DB = db
			common.SetMainDatabaseType(databaseType)
			initCol()
			require.NoError(t, DB.AutoMigrate(&User{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&User{})) })
			insertUsersForPaginationTest(t, 42)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", 1).Updates(map[string]any{
				"quota": 12345, "used_quota": 789,
			}).Error)
			require.NoError(t, DB.AutoMigrate(&User{}))
			var preserved User
			require.NoError(t, DB.First(&preserved, 1).Error)
			assert.Equal(t, 12345, preserved.Quota)
			assert.Equal(t, 789, preserved.UsedQuota)
			require.NoError(t, DB.Delete(&User{}, 2).Error)

			expected := []int{22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41}
			users, total, err := GetAllUsers(&common.PageInfo{Page: 2, PageSize: 20}, NewUserSortOptions("id", "asc"))
			require.NoError(t, err)
			assert.Equal(t, int64(41), total)
			assert.Equal(t, expected, collectUserIDs(users))
			users, total, err = SearchUsers("user", "", nil, nil, 20, 20, NewUserSortOptions("id", "asc"))
			require.NoError(t, err)
			assert.Equal(t, int64(41), total)
			assert.Equal(t, expected, collectUserIDs(users))
			deletedStatus := -1
			users, total, err = SearchUsers("user", "", nil, &deletedStatus, 0, 20, NewUserSortOptions("id", "asc"))
			require.NoError(t, err)
			assert.Equal(t, int64(1), total)
			assert.Equal(t, []int{2}, collectUserIDs(users))
		})
	}
}
