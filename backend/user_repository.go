package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type UserRepository struct {
	collection *mongo.Collection
}

func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{
		collection: db.Collection("users"),
	}
}

func (r *UserRepository) EnsureIndexesAndSeedUsers(ctx context.Context, instrUser, instrPass, traineeUser, traineePass string) error {
	idx := mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := r.collection.Indexes().CreateOne(ctx, idx); err != nil {
		return err
	}

	seeds := []struct {
		username    string
		password    string
		displayName string
		role        string
	}{
		{instrUser, instrPass, "SOC Instructor", "instructor"},
		{traineeUser, traineePass, "SOC Trainee", "trainee"},
	}

	for _, s := range seeds {
		u, err := NormalizeAndValidateUsername(s.username)
		if err != nil || strings.TrimSpace(s.password) == "" {
			continue
		}
		if _, err := r.FindByUsername(ctx, u); err == nil {
			continue
		} else if !errors.Is(err, ErrUserNotFound) {
			return err
		}
		hash, err := HashPassword(s.password)
		if err != nil {
			return err
		}
		now := time.Now()
		doc := UserDocument{
			ID:           GeneratePrefixedID("usr"),
			Username:     u,
			DisplayName:  s.displayName,
			Role:         s.role,
			PasswordHash: hash,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if _, err := r.collection.InsertOne(ctx, doc); err != nil && !mongo.IsDuplicateKeyError(err) {
			return err
		}
	}
	return nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*UserDocument, error) {
	u := strings.ToLower(strings.TrimSpace(username))
	var doc UserDocument
	err := r.collection.FindOne(ctx, bson.M{"username": u}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*UserDocument, error) {
	var doc UserDocument
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (r *UserRepository) Create(ctx context.Context, username, password, displayName, role string) (UserDocument, error) {
	normUser, err := NormalizeAndValidateUsername(username)
	if err != nil {
		return UserDocument{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return UserDocument{}, err
	}
	display := strings.TrimSpace(displayName)
	if display == "" {
		display = normUser
	}
	now := time.Now()
	doc := UserDocument{
		ID:           GeneratePrefixedID("usr"),
		Username:     normUser,
		DisplayName:  display,
		Role:         NormalizeRole(role),
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := r.collection.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return UserDocument{}, ErrUserAlreadyExists
		}
		return UserDocument{}, err
	}
	return doc, nil
}
