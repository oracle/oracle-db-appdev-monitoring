// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
)

type databaseListerFunc func(context.Context, adb.ListAutonomousDatabasesRequest) (adb.ListAutonomousDatabasesResponse, error)

func (f databaseListerFunc) ListAutonomousDatabases(ctx context.Context, req adb.ListAutonomousDatabasesRequest) (adb.ListAutonomousDatabasesResponse, error) {
	return f(ctx, req)
}

func TestListAllDatabasesPaginationAndTags(t *testing.T) {
	calls := 0
	ctx := context.Background()
	client := databaseListerFunc(func(gotCtx context.Context, req adb.ListAutonomousDatabasesRequest) (adb.ListAutonomousDatabasesResponse, error) {
		calls++
		if gotCtx != ctx || req.CompartmentId == nil || *req.CompartmentId != "compartment" || req.LifecycleState != adb.AutonomousDatabaseSummaryLifecycleStateAvailable {
			t.Fatalf("unexpected request: %#v", req)
		}
		if calls == 1 {
			if req.Page != nil {
				t.Fatal("unexpected first page token")
			}
			return adb.ListAutonomousDatabasesResponse{Items: []adb.AutonomousDatabaseSummary{
				{DbName: common.String("match1"), FreeformTags: map[string]string{"enabled": "true", "team": "orders"}},
				{DbName: common.String("partial"), FreeformTags: map[string]string{"enabled": "true"}},
			}, OpcNextPage: common.String("next")}, nil
		}
		if calls != 2 || req.Page == nil || *req.Page != "next" {
			t.Fatalf("unexpected page request: %#v", req)
		}
		return adb.ListAutonomousDatabasesResponse{Items: []adb.AutonomousDatabaseSummary{
			{DbName: common.String("match2"), FreeformTags: map[string]string{"enabled": "true", "team": "orders"}},
		}}, nil
	})
	results, err := ListAllDatabases(ctx, client, "compartment", map[string]string{"enabled": "true", "team": "orders"}, adb.AutonomousDatabaseLifecycleStateAvailable)
	if err != nil || len(results) != 2 || *results[0].DbName != "match1" || *results[1].DbName != "match2" || calls != 2 {
		t.Fatalf("unexpected discovery results: %#v, %v", results, err)
	}
}

func TestListAllDatabasesReturnsPageError(t *testing.T) {
	want := errors.New("list failed")
	calls := 0
	client := databaseListerFunc(func(context.Context, adb.ListAutonomousDatabasesRequest) (adb.ListAutonomousDatabasesResponse, error) {
		calls++
		if calls == 1 {
			return adb.ListAutonomousDatabasesResponse{Items: []adb.AutonomousDatabaseSummary{{}}, OpcNextPage: common.String("next")}, nil
		}
		return adb.ListAutonomousDatabasesResponse{}, want
	})
	results, err := ListAllDatabases(context.Background(), client, "compartment", nil, adb.AutonomousDatabaseLifecycleStateAvailable)
	if !errors.Is(err, want) || results != nil {
		t.Fatalf("expected listing error without partial targets, got %#v, %v", results, err)
	}
}

func TestRequiredTagsRequireKeyEvenForEmptyValue(t *testing.T) {
	if hasRequiredTags(nil, map[string]string{"required": ""}) {
		t.Fatal("absent tag matched empty required value")
	}
	if !hasRequiredTags(map[string]string{"required": ""}, map[string]string{"required": ""}) {
		t.Fatal("present empty tag did not match")
	}
}
