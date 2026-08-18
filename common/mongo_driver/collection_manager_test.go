package mongo_driver

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type collectionTestDocument struct {
	ID bson.ObjectID
}

func TestCollectionObjectIDConversion(t *testing.T) {
	cb := &CollectionBase[collectionTestDocument]{}
	id := bson.NewObjectID()
	converted, err := cb.ToObjectID(id.Hex())
	if err != nil {
		t.Fatalf("ToObjectID() error = %v", err)
	}
	t.Logf("actual: %s", converted.Hex())
	t.Logf("expected: %s", id.Hex())
	if converted != id || cb.ObjectIDToHex(converted) != id.Hex() {
		t.Fatalf("object ID round trip = %s, want %s", converted.Hex(), id.Hex())
	}
	if got, err := cb.ToObjectID("invalid"); !errors.Is(err, ErrInvalidHex) || got != NilObjectID {
		t.Fatalf("invalid ToObjectID() = (%v, %v)", got, err)
	}
}

func TestCollectionInputValidationWithoutDatabase(t *testing.T) {
	cb := &CollectionBase[collectionTestDocument]{}
	filter := NewFilterBuilder().EQ("id", 1).Build()
	update := NewOperatorBuilder().Set(E{Key: "name", Value: "new"}).Build()

	_, actualErr := cb.InsertOne(nil, nil)
	t.Logf("actual: %v", actualErr)
	t.Logf("expected: %v", ErrNilDocument)
	if !errors.Is(actualErr, ErrNilDocument) {
		t.Fatalf("InsertOne(nil) error = %v", actualErr)
	}
	if _, err := cb.InsertMany(nil, nil); !errors.Is(err, ErrNilDocument) {
		t.Fatalf("InsertMany(nil) error = %v", err)
	}
	if _, err := cb.FindOne(nil, nil); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("FindOne(nil) error = %v", err)
	}
	if _, err := cb.FineCursor(nil, nil); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("FineCursor(nil) error = %v", err)
	}
	if _, err := cb.FindAll(nil, nil); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("FindAll(nil) error = %v", err)
	}
	if _, err := cb.UpdateOne(nil, nil, update); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("UpdateOne(nil filter) error = %v", err)
	}
	if _, err := cb.UpdateOne(nil, filter, nil); !errors.Is(err, ErrNoDocuments) {
		t.Fatalf("UpdateOne(nil update) error = %v", err)
	}
	if _, err := cb.UpdateMany(nil, nil, update); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("UpdateMany(nil filter) error = %v", err)
	}
	if _, err := cb.UpdateMany(nil, filter, nil); !errors.Is(err, ErrNoDocuments) {
		t.Fatalf("UpdateMany(nil update) error = %v", err)
	}
	if _, err := cb.DeleteOne(nil, nil); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("DeleteOne(nil) error = %v", err)
	}
	if _, err := cb.DeleteMany(nil, nil); !errors.Is(err, ErrFindFilterNil) {
		t.Fatalf("DeleteMany(nil) error = %v", err)
	}
	if _, err := cb.UpdateOneByID(nil, "invalid", update); !errors.Is(err, ErrInvalidHex) {
		t.Fatalf("UpdateOneByID(invalid) error = %v", err)
	}
	if _, err := cb.DeleteOneByID(nil, "invalid"); !errors.Is(err, ErrInvalidHex) {
		t.Fatalf("DeleteOneByID(invalid) error = %v", err)
	}
}

func TestMongoManagersWithoutDatabaseConnection(t *testing.T) {
	manager := NewMongoDBManager("node-a")
	t.Logf("actual: name=%q, client=%v", manager.Name(), manager.Client())
	t.Logf("expected: name=%q, client=<nil>", "node-a")
	if manager.Name() != "node-a" || manager.Client() != nil {
		t.Fatalf("new manager = name %q, client %v", manager.Name(), manager.Client())
	}
	if err := manager.InitConfiguration(nil); !errors.Is(err, ErrInitInvalidConfig) {
		t.Fatalf("InitConfiguration(nil) error = %v", err)
	}

	cluster := NewMongoClusterManager()
	if cluster.GetMongoDBManager("missing") != nil {
		t.Fatal("missing node returned a manager")
	}
	cluster.AddNodeMongoDBManager("node-a", manager)
	if cluster.GetMongoDBManager("node-a") != manager {
		t.Fatal("stored manager was not returned")
	}
	if err := cluster.Destroy(nil); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
}

func TestMongoGlobalAndCollectionValidation(t *testing.T) {
	actualErr := InitConfiguration(nil)
	t.Logf("actual: %v", actualErr)
	t.Logf("expected: %v", ErrInitInvalidConfig)
	if !errors.Is(actualErr, ErrInitInvalidConfig) {
		t.Fatalf("InitConfiguration(nil) error = %v", actualErr)
	}
	if GetMongoDBManager("missing") != nil {
		t.Fatal("global missing manager was not nil")
	}
	if err := Destroy(nil); err != nil {
		t.Fatalf("Destroy() without initialization error = %v", err)
	}
	if _, err := CreateCollectionBase[collectionTestDocument]("missing", "db", "table"); err == nil {
		t.Fatal("CreateCollectionBase() with missing manager returned nil error")
	}
}
