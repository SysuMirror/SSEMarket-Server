package service_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
	"loginTest/service"
	"loginTest/util"

	"github.com/redis/go-redis/v9"
)

// modifyEmailRepository embeds the production interface so each test only
// needs to define the methods used by ModifyEmail.
type modifyEmailRepository struct {
	repository.UserRepository

	user       *domain.User
	findErr    error
	exists     bool
	existsErr  error
	updateErr  error
	updateID   int
	updateOld  string
	updateMail string
	updateN    int
	history    []string
}

func (r *modifyEmailRepository) FindByID(context.Context, int) (*domain.User, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	if r.user == nil {
		return nil, domain.ErrNotFound
	}
	return r.user, nil
}

func (r *modifyEmailRepository) ExistsByEmail(context.Context, string) (bool, error) {
	return r.exists, r.existsErr
}

func (r *modifyEmailRepository) UpdateEmailWithHistory(_ context.Context, userID int, oldEmail, email string) error {
	r.updateN++
	r.updateID = userID
	r.updateOld = oldEmail
	r.updateMail = email
	if r.updateErr == nil && strings.TrimSpace(oldEmail) != "" {
		r.history = append(r.history, strings.ToLower(strings.TrimSpace(oldEmail)))
	}
	return r.updateErr
}

type inMemoryRedis struct {
	mu     sync.Mutex
	values map[string]string
	fail   bool
}

func newInMemoryRedis(t *testing.T) (*redis.Client, *inMemoryRedis) {
	t.Helper()
	store := &inMemoryRedis{values: make(map[string]string)}
	client := redis.NewClient(&redis.Options{
		Protocol: 2,
		PoolSize: 1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go store.serve(serverConn)
			return clientConn, nil
		},
	})
	t.Cleanup(func() { _ = client.Close() })
	return client, store
}

func (r *inMemoryRedis) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		args, err := readRESPCommand(reader)
		if err != nil {
			return
		}
		if len(args) == 0 {
			_, _ = io.WriteString(conn, "-ERR empty command\r\n")
			continue
		}
		if r.fail {
			_, _ = io.WriteString(conn, "-ERR unavailable\r\n")
			continue
		}
		switch strings.ToUpper(args[0]) {
		case "HELLO":
			_, _ = io.WriteString(conn, "%2\r\n+server\r\n+redis\r\n+proto\r\n:2\r\n")
		case "GET":
			r.mu.Lock()
			value, ok := r.values[args[1]]
			r.mu.Unlock()
			if !ok {
				_, _ = io.WriteString(conn, "$-1\r\n")
				continue
			}
			_, _ = fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(value), value)
		case "SET":
			r.mu.Lock()
			r.values[args[1]] = args[2]
			r.mu.Unlock()
			_, _ = io.WriteString(conn, "+OK\r\n")
		case "DEL":
			deleted := 0
			r.mu.Lock()
			for _, key := range args[1:] {
				if _, ok := r.values[key]; ok {
					delete(r.values, key)
					deleted++
				}
			}
			r.mu.Unlock()
			_, _ = fmt.Fprintf(conn, ":%d\r\n", deleted)
		default:
			// go-redis may issue connection setup commands depending on its
			// version. A successful simple reply is sufficient for this test.
			_, _ = io.WriteString(conn, "+OK\r\n")
		}
	}
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 4 || line[0] != '*' {
		return nil, fmt.Errorf("invalid command header %q", line)
	}
	count, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(line[1:], "\n"), "\r"))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		lengthLine, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(lengthLine, "$"), "\n"), "\r"))
		if err != nil {
			return nil, err
		}
		data := make([]byte, length+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		args = append(args, string(data[:length]))
	}
	return args, nil
}

func withRedis(t *testing.T) *inMemoryRedis {
	t.Helper()
	previous := core.MyRedis
	client, store := newInMemoryRedis(t)
	core.MyRedis = client
	t.Cleanup(func() { core.MyRedis = previous })
	return store
}

func TestModifyEmailRejectsInvalidInputBeforeRedis(t *testing.T) {
	t.Cleanup(func() { core.MyRedis = nil })
	repo := &modifyEmailRepository{user: &domain.User{ID: 7, Email: "old@example.com"}}
	svc := service.NewUserService(repo)

	tests := []struct {
		name string
		in   domain.ModifyEmailInput
		want error
	}{
		{name: "missing user", in: domain.ModifyEmailInput{NewEmail: "new@example.com"}, want: domain.ErrUnauthorized},
		{name: "missing local part", in: domain.ModifyEmailInput{UserID: 7, NewEmail: "@example.com"}, want: domain.ErrBadRequest},
		{name: "multiple at signs", in: domain.ModifyEmailInput{UserID: 7, NewEmail: "new@@example.com"}, want: domain.ErrBadRequest},
		{name: "missing domain", in: domain.ModifyEmailInput{UserID: 7, NewEmail: "new@"}, want: domain.ErrBadRequest},
		{name: "same email", in: domain.ModifyEmailInput{UserID: 7, NewEmail: " OLD@example.com "}, want: domain.ErrBadRequest},
		{name: "overlong email", in: domain.ModifyEmailInput{UserID: 7, NewEmail: strings.Repeat("a", 250) + "@example.com"}, want: domain.ErrBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ModifyEmail(context.Background(), tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ModifyEmail error = %v, want %v", err, tt.want)
			}
			if repo.updateN != 0 {
				t.Fatalf("UpdateEmailWithHistory called %d times", repo.updateN)
			}
		})
	}
}

func TestModifyEmailPropagatesRepositoryErrors(t *testing.T) {
	withRedis(t)

	findErr := errors.New("find failed")
	repo := &modifyEmailRepository{findErr: findErr}
	_, err := service.NewUserService(repo).ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
	})
	if !errors.Is(err, findErr) {
		t.Fatalf("FindByID error = %v, want %v", err, findErr)
	}

	repo = &modifyEmailRepository{}
	_, err = service.NewUserService(repo).ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing user error = %v, want ErrNotFound", err)
	}

	existsErr := errors.New("exists failed")
	repo = &modifyEmailRepository{user: &domain.User{ID: 7, Email: "old@example.com"}, existsErr: existsErr}
	_, err = service.NewUserService(repo).ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
	})
	if !errors.Is(err, existsErr) {
		t.Fatalf("ExistsByEmail error = %v, want %v", err, existsErr)
	}
}

func TestModifyEmailRedisUnavailable(t *testing.T) {
	previous := core.MyRedis
	core.MyRedis = nil
	t.Cleanup(func() { core.MyRedis = previous })
	repo := &modifyEmailRepository{user: &domain.User{ID: 7, Email: "old@example.com"}}
	_, err := service.NewUserService(repo).ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
	})
	if !errors.Is(err, domain.ErrInternal) {
		t.Fatalf("Redis-unavailable error = %v, want ErrInternal", err)
	}
	if repo.updateN != 0 {
		t.Fatalf("UpdateEmailWithHistory called %d times", repo.updateN)
	}
}

func TestModifyEmailRejectsExistingEmailAndBadCode(t *testing.T) {
	store := withRedis(t)
	repo := &modifyEmailRepository{user: &domain.User{ID: 7, Email: "old@example.com"}, exists: true}
	svc := service.NewUserService(repo)
	_, err := svc.ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("existing email error = %v, want ErrConflict", err)
	}
	if repo.updateN != 0 {
		t.Fatalf("UpdateEmailWithHistory called %d times for existing email", repo.updateN)
	}

	repo.exists = false
	store.values["vcode:new@example.com"] = "654321"
	for i := 0; i < 5; i++ {
		_, err = svc.ModifyEmail(context.Background(), domain.ModifyEmailInput{
			UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
		})
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("bad code attempt %d error = %v, want ErrUnauthorized", i+1, err)
		}
	}
	store.mu.Lock()
	_, codeStillPresent := store.values["vcode:new@example.com"]
	store.mu.Unlock()
	if codeStillPresent {
		t.Fatal("verification code remains after five failed attempts")
	}
	if repo.updateN != 0 {
		t.Fatalf("UpdateEmailWithHistory called %d times for bad code", repo.updateN)
	}
}

func TestModifyEmailSuccessNormalizesAndInvalidatesCaches(t *testing.T) {
	store := withRedis(t)
	user := &domain.User{ID: 7, Email: "old@example.com"}
	repo := &modifyEmailRepository{user: user}
	store.values["vcode:new@example.com"] = "123456"
	store.values["vcode_fail:new@example.com"] = "2"
	store.values[util.PrefixUserSession+"7"] = "session"
	store.values[util.GetUserInfoKey(7)] = "profile"

	got, err := service.NewUserService(repo).ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "  New@Example.COM ", ValiCode: "123456",
	})
	if err != nil {
		t.Fatalf("ModifyEmail: %v", err)
	}
	if got != user || got.Email != "new@example.com" {
		t.Fatalf("returned user = %+v, want normalized email", got)
	}
	if repo.updateN != 1 || repo.updateID != 7 || repo.updateOld != "old@example.com" || repo.updateMail != "new@example.com" {
		t.Fatalf("update = (%d, %d, %q, %q), want (1, 7, old@example.com, new@example.com)", repo.updateN, repo.updateID, repo.updateOld, repo.updateMail)
	}
	if len(repo.history) != 1 || repo.history[0] != "old@example.com" {
		t.Fatalf("history = %v, want [old@example.com]", repo.history)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	for _, key := range []string{
		"vcode:new@example.com",
		"vcode_fail:new@example.com",
		util.PrefixUserSession + "7",
		util.GetUserInfoKey(7),
	} {
		if _, ok := store.values[key]; ok {
			t.Errorf("cache key %q was not invalidated", key)
		}
	}
}

func TestModifyEmailDatabaseUpdateFailureKeepsVerificationCode(t *testing.T) {
	store := withRedis(t)
	updateErr := errors.New("update failed")
	repo := &modifyEmailRepository{
		user:      &domain.User{ID: 7, Email: "old@example.com"},
		updateErr: updateErr,
	}
	store.values["vcode:new@example.com"] = "123456"

	_, err := service.NewUserService(repo).ModifyEmail(context.Background(), domain.ModifyEmailInput{
		UserID: 7, NewEmail: "new@example.com", ValiCode: "123456",
	})
	if !errors.Is(err, updateErr) {
		t.Fatalf("UpdateEmailWithHistory error = %v, want %v", err, updateErr)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.values["vcode:new@example.com"]; !ok {
		t.Fatal("verification code was cleared after database update failure")
	}
	if len(repo.history) != 0 {
		t.Fatalf("history = %v, want no history after update failure", repo.history)
	}
}
