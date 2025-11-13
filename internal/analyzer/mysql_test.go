package analyzer

import (
	"testing"
)

// TestParseMySQLExplain tests parsing of MySQL EXPLAIN FORMAT=JSON output.
func TestParseMySQLExplain(t *testing.T) {
	tests := []struct {
		name        string
		jsonInput   string
		withAnalyze bool
		want        *QueryPlan
		wantErr     bool
	}{
		{
			name: "index_scan",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"cost_info": {
						"query_cost": "1.20"
					},
					"table": {
						"table_name": "users",
						"access_type": "ref",
						"possible_keys": ["email_idx"],
						"key": "email_idx",
						"used_key_parts": ["email"],
						"key_length": "768",
						"ref": ["const"],
						"rows_examined_per_scan": 1,
						"rows_produced_per_join": 1,
						"filtered": 100.00,
						"cost_info": {
							"read_cost": "1.00",
							"eval_cost": "0.20",
							"prefix_cost": "1.20"
						}
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          1.20,
				EstimatedRows: 1,
				UsesIndex:     true,
				IndexName:     "email_idx",
				FullScan:      false,
				Database:      "mysql",
				RowsExamined:  1,
				RowsProduced:  1,
			},
			wantErr: false,
		},
		{
			name: "full_table_scan",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"cost_info": {
						"query_cost": "250.50"
					},
					"table": {
						"table_name": "users",
						"access_type": "ALL",
						"possible_keys": null,
						"key": "",
						"rows_examined_per_scan": 1000,
						"rows_produced_per_join": 1000,
						"filtered": 100.00,
						"cost_info": {
							"read_cost": "200.00",
							"eval_cost": "50.50",
							"prefix_cost": "250.50"
						}
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          250.50,
				EstimatedRows: 1000,
				UsesIndex:     false,
				IndexName:     "",
				FullScan:      true,
				Database:      "mysql",
				RowsExamined:  1000,
				RowsProduced:  1000,
			},
			wantErr: false,
		},
		{
			name: "nested_loop_join",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"cost_info": {
						"query_cost": "150.75"
					},
					"nested_loop": {
						"table": [
							{
								"table_name": "users",
								"access_type": "ALL",
								"key": "",
								"rows_examined_per_scan": 100,
								"rows_produced_per_join": 100,
								"cost_info": {
									"read_cost": "50.00"
								}
							},
							{
								"table_name": "orders",
								"access_type": "ref",
								"key": "user_id_idx",
								"rows_examined_per_scan": 10,
								"rows_produced_per_join": 1000,
								"cost_info": {
									"read_cost": "100.75"
								}
							}
						]
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          150.75,
				EstimatedRows: 110, // 100 + 10
				UsesIndex:     true,
				IndexName:     "user_id_idx",
				FullScan:      true, // users table has full scan
				Database:      "mysql",
				RowsExamined:  110,
				RowsProduced:  1100, // 100 + 1000
			},
			wantErr: false,
		},
		{
			name: "grouping_operation",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"cost_info": {
						"query_cost": "75.30"
					},
					"grouping_operation": {
						"using_temporary_table": true,
						"using_filesort": true,
						"table": {
							"table_name": "orders",
							"access_type": "index",
							"key": "user_id_idx",
							"rows_examined_per_scan": 500,
							"rows_produced_per_join": 500,
							"cost_info": {
								"read_cost": "75.30"
							}
						}
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          75.30,
				EstimatedRows: 500,
				UsesIndex:     true,
				IndexName:     "user_id_idx",
				FullScan:      false,
				Database:      "mysql",
				RowsExamined:  500,
				RowsProduced:  500,
			},
			wantErr: false,
		},
		{
			name: "ordering_operation",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"cost_info": {
						"query_cost": "120.45"
					},
					"ordering_operation": {
						"using_filesort": true,
						"table": {
							"table_name": "users",
							"access_type": "range",
							"key": "status_idx",
							"rows_examined_per_scan": 200,
							"rows_produced_per_join": 200,
							"cost_info": {
								"read_cost": "120.45"
							}
						}
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          120.45,
				EstimatedRows: 200,
				UsesIndex:     true,
				IndexName:     "status_idx",
				FullScan:      false,
				Database:      "mysql",
				RowsExamined:  200,
				RowsProduced:  200,
			},
			wantErr: false,
		},
		{
			name: "empty_json",
			jsonInput: `{
				"query_block": {
					"select_id": 1
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 0,
				UsesIndex:     false,
				FullScan:      false,
				Database:      "mysql",
			},
			wantErr: false,
		},
		{
			name: "missing_cost_info",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"table": {
						"table_name": "users",
						"access_type": "ALL",
						"rows_examined_per_scan": 100,
						"rows_produced_per_join": 100
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 100,
				UsesIndex:     false,
				FullScan:      true,
				Database:      "mysql",
				RowsExamined:  100,
				RowsProduced:  100,
			},
			wantErr: false,
		},
		{
			name: "multiple_indexes",
			jsonInput: `{
				"query_block": {
					"select_id": 1,
					"nested_loop": {
						"table": [
							{
								"table_name": "t1",
								"access_type": "ref",
								"key": "idx1",
								"rows_examined_per_scan": 10,
								"rows_produced_per_join": 10
							},
							{
								"table_name": "t2",
								"access_type": "ref",
								"key": "idx2",
								"rows_examined_per_scan": 20,
								"rows_produced_per_join": 20
							}
						]
					}
				}
			}`,
			withAnalyze: false,
			want: &QueryPlan{
				Cost:          0,
				EstimatedRows: 30, // 10 + 20
				UsesIndex:     true,
				IndexName:     "idx1", // First index encountered
				FullScan:      false,
				Database:      "mysql",
				RowsExamined:  30,
				RowsProduced:  30,
			},
			wantErr: false,
		},
		{
			name:        "invalid_json",
			jsonInput:   `{invalid json`,
			withAnalyze: false,
			want:        nil,
			wantErr:     true,
		},
		{
			name:        "empty_string",
			jsonInput:   ``,
			withAnalyze: false,
			want:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMySQLExplain(tt.jsonInput, tt.withAnalyze)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseMySQLExplain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Compare key fields
			if got.Cost != tt.want.Cost {
				t.Errorf("Cost = %.2f, want %.2f", got.Cost, tt.want.Cost)
			}

			if got.EstimatedRows != tt.want.EstimatedRows {
				t.Errorf("EstimatedRows = %d, want %d", got.EstimatedRows, tt.want.EstimatedRows)
			}

			if got.UsesIndex != tt.want.UsesIndex {
				t.Errorf("UsesIndex = %v, want %v", got.UsesIndex, tt.want.UsesIndex)
			}

			if got.IndexName != tt.want.IndexName {
				t.Errorf("IndexName = %q, want %q", got.IndexName, tt.want.IndexName)
			}

			if got.FullScan != tt.want.FullScan {
				t.Errorf("FullScan = %v, want %v", got.FullScan, tt.want.FullScan)
			}

			if got.Database != tt.want.Database {
				t.Errorf("Database = %q, want %q", got.Database, tt.want.Database)
			}

			if got.RowsExamined != tt.want.RowsExamined {
				t.Errorf("RowsExamined = %d, want %d", got.RowsExamined, tt.want.RowsExamined)
			}

			if got.RowsProduced != tt.want.RowsProduced {
				t.Errorf("RowsProduced = %d, want %d", got.RowsProduced, tt.want.RowsProduced)
			}
		})
	}
}

func TestParseFloatOrZero(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		hasError bool
	}{
		{"123.45", 123.45, false},
		{"0", 0, false},
		{"", 0, false},
		{"1.20", 1.20, false},
		{"250.50", 250.50, false},
		{"75.30", 75.30, false},
		{"120.45", 120.45, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseFloatOrZero(tt.input)
			if (err != nil) != tt.hasError {
				t.Errorf("parseFloatOrZero(%q) error = %v, hasError = %v", tt.input, err, tt.hasError)
			}
			if result != tt.expected {
				t.Errorf("parseFloatOrZero(%q) = %.2f, expected %.2f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUpdateMySQLTableMetrics(t *testing.T) {
	t.Run("nil_table", func(t *testing.T) {
		plan := &QueryPlan{}
		updateMySQLTableMetrics(nil, plan)
		// Should not panic
		if plan.UsesIndex {
			t.Error("Expected no changes for nil table")
		}
	})

	t.Run("table_with_index", func(t *testing.T) {
		plan := &QueryPlan{}
		table := &mysqlTableAccess{
			TableName:            "users",
			AccessType:           "ref",
			Key:                  "email_idx",
			RowsExaminedPerScan:  10,
			RowsProducedPerJoin:  10,
		}

		updateMySQLTableMetrics(table, plan)

		if !plan.UsesIndex {
			t.Error("Expected UsesIndex = true")
		}

		if plan.IndexName != "email_idx" {
			t.Errorf("Expected IndexName = 'email_idx', got %q", plan.IndexName)
		}

		if plan.FullScan {
			t.Error("Expected FullScan = false for ref access")
		}

		if plan.EstimatedRows != 10 {
			t.Errorf("Expected EstimatedRows = 10, got %d", plan.EstimatedRows)
		}
	})

	t.Run("table_full_scan", func(t *testing.T) {
		plan := &QueryPlan{}
		table := &mysqlTableAccess{
			TableName:            "users",
			AccessType:           "ALL",
			Key:                  "",
			RowsExaminedPerScan:  1000,
			RowsProducedPerJoin:  1000,
		}

		updateMySQLTableMetrics(table, plan)

		if plan.UsesIndex {
			t.Error("Expected UsesIndex = false")
		}

		if !plan.FullScan {
			t.Error("Expected FullScan = true for ALL access")
		}

		if plan.EstimatedRows != 1000 {
			t.Errorf("Expected EstimatedRows = 1000, got %d", plan.EstimatedRows)
		}
	})

	t.Run("accumulate_metrics", func(t *testing.T) {
		plan := &QueryPlan{
			EstimatedRows: 50,
			RowsExamined:  50,
			RowsProduced:  50,
		}
		table := &mysqlTableAccess{
			TableName:            "orders",
			AccessType:           "ref",
			Key:                  "idx",
			RowsExaminedPerScan:  25,
			RowsProducedPerJoin:  25,
		}

		updateMySQLTableMetrics(table, plan)

		if plan.EstimatedRows != 75 {
			t.Errorf("Expected EstimatedRows = 75, got %d", plan.EstimatedRows)
		}

		if plan.RowsExamined != 75 {
			t.Errorf("Expected RowsExamined = 75, got %d", plan.RowsExamined)
		}

		if plan.RowsProduced != 75 {
			t.Errorf("Expected RowsProduced = 75, got %d", plan.RowsProduced)
		}
	})
}

func TestExtractMySQLMetrics(t *testing.T) {
	t.Run("nested_loop_with_grouping", func(t *testing.T) {
		queryBlock := &mysqlQueryBlock{
			SelectID: 1,
			CostInfo: mysqlCostInfo{
				QueryCost: "200.00",
			},
			Grouping: &mysqlGrouping{
				UsingTemporaryTable: true,
				UsingFilesort:       true,
				NestedLoop: &mysqlNestedLoop{
					Table: []*mysqlTableAccess{
						{
							TableName:            "users",
							AccessType:           "ALL",
							Key:                  "",
							RowsExaminedPerScan:  100,
							RowsProducedPerJoin:  100,
						},
						{
							TableName:            "orders",
							AccessType:           "ref",
							Key:                  "user_id_idx",
							RowsExaminedPerScan:  50,
							RowsProducedPerJoin:  50,
						},
					},
				},
			},
		}

		plan := &QueryPlan{}
		extractMySQLMetrics(queryBlock, plan)

		if !plan.UsesIndex {
			t.Error("Expected UsesIndex = true")
		}

		if !plan.FullScan {
			t.Error("Expected FullScan = true (users table)")
		}

		if plan.EstimatedRows != 150 {
			t.Errorf("Expected EstimatedRows = 150, got %d", plan.EstimatedRows)
		}
	})

	t.Run("ordering_with_nested_loop", func(t *testing.T) {
		queryBlock := &mysqlQueryBlock{
			SelectID: 1,
			Ordering: &mysqlOrdering{
				UsingFilesort: true,
				NestedLoop: &mysqlNestedLoop{
					Table: []*mysqlTableAccess{
						{
							TableName:            "t1",
							AccessType:           "index",
							Key:                  "idx1",
							RowsExaminedPerScan:  30,
							RowsProducedPerJoin:  30,
						},
						{
							TableName:            "t2",
							AccessType:           "index",
							Key:                  "idx2",
							RowsExaminedPerScan:  40,
							RowsProducedPerJoin:  40,
						},
					},
				},
			},
		}

		plan := &QueryPlan{}
		extractMySQLMetrics(queryBlock, plan)

		if !plan.UsesIndex {
			t.Error("Expected UsesIndex = true")
		}

		if plan.EstimatedRows != 70 {
			t.Errorf("Expected EstimatedRows = 70, got %d", plan.EstimatedRows)
		}
	})
}
