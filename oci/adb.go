// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"context"

	adb "github.com/oracle/oci-go-sdk/v65/database"
)

// ListAllDatabases returns the Autonomous Databases in a compartment that have
// every required freeform tag and match the requested lifecycle state.
func ListAllDatabases(ctx context.Context, client adb.DatabaseClient, compartmentId string, requiredTags map[string]string, lifecycleState adb.AutonomousDatabaseLifecycleStateEnum) ([]adb.AutonomousDatabaseSummary, error) {
	request := adb.ListAutonomousDatabasesRequest{
		CompartmentId:  &compartmentId,
		LifecycleState: adb.AutonomousDatabaseSummaryLifecycleStateEnum(lifecycleState),
	}

	var databases []adb.AutonomousDatabaseSummary
	for {
		response, err := client.ListAutonomousDatabases(ctx, request)
		if err != nil {
			return nil, err
		}
		for _, database := range response.Items {
			if hasRequiredTags(database.FreeformTags, requiredTags) {
				databases = append(databases, database)
			}
		}
		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			return databases, nil
		}
		request.Page = response.OpcNextPage
	}
}

func hasRequiredTags(tags, requiredTags map[string]string) bool {
	for key, requiredValue := range requiredTags {
		if tags[key] != requiredValue {
			return false
		}
	}
	return true
}
