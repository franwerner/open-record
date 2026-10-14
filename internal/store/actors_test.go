package store

import (
	"testing"
)

func TestLoadActorsReportsAnUndeclaredStoreAsBenign(t *testing.T) {
	repo := t.TempDir()
	_, err := LoadActors(repo)
	if err == nil {
		t.Fatal("LoadActors on an empty repo returned no error")
	}
	if !IsActorsUndeclared(err) {
		t.Errorf("IsActorsUndeclared(%v) = false, want true", err)
	}
}

func TestActorsSaveAndLoadRoundTrip(t *testing.T) {
	repo := t.TempDir()
	set := Actors{Actors: []Actor{
		{ID: "visitor", Kind: ActorHuman, Description: "d", Aliases: []string{"guest"}},
	}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadActors(repo)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Hash == "" {
		t.Error("Save did not stamp a hash")
	}
	if findings := VerifyActors(repo, reloaded); len(findings) != 0 {
		t.Errorf("VerifyActors rejected what Save wrote: %+v", findings)
	}
	if !reloaded.Has("visitor") || !reloaded.Has("VISITOR") {
		t.Error("Has is not case-insensitive")
	}
	actor, ok := reloaded.Get("visitor")
	if !ok || actor.Description != "d" || len(actor.Aliases) != 1 || actor.Aliases[0] != "guest" {
		t.Errorf("Get returned %+v, ok=%v", actor, ok)
	}
}

func TestActorsValidateReportsADuplicateID(t *testing.T) {
	set := Actors{Actors: []Actor{
		{ID: "visitor", Kind: ActorHuman, Description: "a"},
		{ID: "Visitor", Kind: ActorHuman, Description: "b"},
	}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "actor-duplicate" {
		t.Fatalf("Validate(duplicate id) = %+v, want exactly one actor-duplicate", findings)
	}
}

func TestActorsValidateReportsAnAliasClaimedByTwoActors(t *testing.T) {
	set := Actors{Actors: []Actor{
		{ID: "visitor", Kind: ActorHuman, Description: "a", Aliases: []string{"guest"}},
		{ID: "administrator", Kind: ActorHuman, Description: "b", Aliases: []string{"Guest"}},
	}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "actor-alias-duplicate" {
		t.Fatalf("Validate(alias collision) = %+v, want exactly one actor-alias-duplicate", findings)
	}
}

func TestActorsValidateReportsAnInvalidKind(t *testing.T) {
	set := Actors{Actors: []Actor{
		{ID: "robot", Kind: "robot", Description: "a"},
	}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "actor-invalid-kind" {
		t.Fatalf("Validate(invalid kind) = %+v, want exactly one actor-invalid-kind", findings)
	}
}

func TestActorsValidateAcceptsAWellFormedDeclaration(t *testing.T) {
	set := Actors{Actors: []Actor{
		{ID: "visitor", Kind: ActorHuman, Description: "a", Aliases: []string{"guest"}},
		{ID: "scheduler", Kind: ActorTime, Description: "b"},
		{ID: "payment-provider", Kind: ActorExternalSystem, Description: "c"},
	}}
	if findings := set.Validate(); len(findings) != 0 {
		t.Errorf("a well-formed declaration was rejected: %+v", findings)
	}
}

func TestActorsIDsAreSorted(t *testing.T) {
	set := Actors{Actors: []Actor{
		{ID: "visitor", Kind: ActorHuman, Description: "a"},
		{ID: "administrator", Kind: ActorHuman, Description: "b"},
	}}
	ids := set.IDs()
	if len(ids) != 2 || ids[0] != "administrator" || ids[1] != "visitor" {
		t.Errorf("IDs = %v, want sorted [administrator visitor]", ids)
	}
}
