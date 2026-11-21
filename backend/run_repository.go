package main

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type RunRepository struct {
	runs   *mongo.Collection
	events *mongo.Collection
}

func NewRunRepository(db *mongo.Database) *RunRepository {
	return &RunRepository{
		runs:   db.Collection("runs"),
		events: db.Collection("run_events"),
	}
}

func (r *RunRepository) EnsureIndexes(ctx context.Context) error {
	runIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "trainee_id", Value: 1}, {Key: "started_at", Value: -1}}},
		{Keys: bson.D{{Key: "scenario_id", Value: 1}, {Key: "started_at", Value: -1}}},
	}
	if _, err := r.runs.Indexes().CreateMany(ctx, runIndexes); err != nil {
		return err
	}
	eventIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "run_id", Value: 1}, {Key: "seq", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := r.events.Indexes().CreateOne(ctx, eventIndex)
	return err
}

func (r *RunRepository) CreateRun(ctx context.Context, run SimulationRun) (SimulationRun, error) {
	if _, err := r.runs.InsertOne(ctx, run); err != nil {
		return SimulationRun{}, err
	}
	return run, nil
}

func (r *RunRepository) GetRunByID(ctx context.Context, id string) (*SimulationRun, error) {
	var run SimulationRun
	err := r.runs.FindOne(ctx, bson.M{"_id": id}).Decode(&run)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (r *RunRepository) UpdateRun(ctx context.Context, run SimulationRun) error {
	res, err := r.runs.ReplaceOne(ctx, bson.M{"_id": run.ID}, run)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrRunNotFound
	}
	return nil
}

func (r *RunRepository) ListRuns(ctx context.Context, traineeID string, limit int64) ([]SimulationRun, error) {
	filter := bson.M{}
	if traineeID != "" {
		filter["trainee_id"] = traineeID
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(limit)
	cursor, err := r.runs.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []SimulationRun{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RunRepository) AppendEvent(ctx context.Context, ev RunEvent) (RunEvent, error) {
	count, err := r.events.CountDocuments(ctx, bson.M{"run_id": ev.RunID})
	if err != nil {
		return RunEvent{}, err
	}
	ev.Seq = int(count) + 1
	if ev.ID == "" {
		ev.ID = GeneratePrefixedID("evt")
	}
	if _, err := r.events.InsertOne(ctx, ev); err != nil {
		return RunEvent{}, err
	}
	return ev, nil
}

func (r *RunRepository) ListEventsByRun(ctx context.Context, runID string) ([]RunEvent, error) {
	opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}})
	cursor, err := r.events.Find(ctx, bson.M{"run_id": runID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []RunEvent{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}
