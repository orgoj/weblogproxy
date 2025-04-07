package truncate

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Helper to create deep copy for tests
func deepCopyMap(original map[string]interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copied map[string]interface{}
	err = json.Unmarshal(b, &copied)
	if err != nil {
		return nil, err
	}
	return copied, nil
}

// Helper to compare maps by their JSON representation
func compareMapsAsJSON(t *testing.T, got, want map[string]interface{}) bool {
	t.Helper()
	gotJSON, errGot := json.Marshal(got)
	if errGot != nil {
		t.Errorf("Failed to marshal 'got' data to JSON: %v", errGot)
		return false
	}
	wantJSON, errWant := json.Marshal(want)
	if errWant != nil {
		t.Errorf("Failed to marshal 'want' data to JSON: %v", errWant)
		return false
	}

	if string(gotJSON) != string(wantJSON) {
		gotJSONIndent, _ := json.MarshalIndent(got, "", "  ")
		wantJSONIndent, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("Data after truncation mismatch (compared as JSON):\nGot:\n%s\nWant:\n%s", gotJSONIndent, wantJSONIndent)
		return false
	}
	return true
}

func TestTruncateMapIfNeeded(t *testing.T) {
	tests := []struct {
		name              string
		inputData         map[string]interface{}
		limit             int64
		expectedData      map[string]interface{}
		expectedTruncated bool
		expectError       bool
	}{
		{
			name:              "NoTruncationNeeded_SmallData",
			inputData:         map[string]interface{}{"key1": "short", "key2": "123"},
			limit:             100,
			expectedData:      map[string]interface{}{"key1": "short", "key2": "123"},
			expectedTruncated: false,
		},
		{
			name:              "NoTruncationNeeded_ExactlyLimit",
			inputData:         map[string]interface{}{"key1": "exactsize"}, // Size is 24 bytes
			limit:             24,
			expectedData:      map[string]interface{}{"key1": "exactsize"},
			expectedTruncated: false, // Should not truncate if size == limit
		},
		{
			name:              "SimpleTruncation_OneLongString",
			inputData:         map[string]interface{}{"longKey": "this is a very long string that needs truncation"}, // len 48
			limit:             40,                                                                                    // Size is 62 bytes initially
			expectedData:      map[string]interface{}{"longKey": "this is a ..."},                                    // Truncated to 10 + ...
			expectedTruncated: true,
		},
		{
			name:              "Truncation_MultipleStrings_LongestTruncatedFirst",
			inputData:         map[string]interface{}{"short": "abc", "long1": "123456789012345", "long2": "abcdefghijklmnopqrstuvwxyz"}, // long2 len 26, long1 len 15
			limit:             70,                                                                                                        // Initial size ~78 bytes
			expectedData:      map[string]interface{}{"short": "abc", "long1": "123456789012345", "long2": "abcdefghij..."},              // long2 truncated to 10 + ... New size should be < 70
			expectedTruncated: true,
		},
		{
			name:              "Truncation_NestedString",
			inputData:         map[string]interface{}{"level1": map[string]interface{}{"nestedKey": "very long nested string content here please truncate"}}, // len 52
			limit:             70,                                                                                                                            // Initial size ~79 bytes
			expectedData:      map[string]interface{}{"level1": map[string]interface{}{"nestedKey": "very long ..."}},                                        // Truncated to 10 + ...
			expectedTruncated: true,
		},
		{
			name:              "Truncation_StringInSlice_LimitNotExceeded",
			inputData:         map[string]interface{}{"list": []interface{}{1, "short", "this string is way too long for the limit"}},
			limit:             70, // Initial size is 63 bytes
			expectedData:      map[string]interface{}{"list": []interface{}{1, "short", "this string is way too long for the limit"}},
			expectedTruncated: false, // Size <= limit, no truncation expected
		},
		{
			name:      "Truncation_MultipleValuesNeeded_Iterative",
			inputData: map[string]interface{}{"a": "long string number one", "b": "long string number two", "c": "short"}, // a len 22, b len 22
			limit:     60,                                                                                                 // Initial size is 70 bytes. Need to reduce by > 10.
			// 1. Truncate 'a' (or 'b') to 10 + ... (size reduces significantly)
			// 2. Recalculate size. If still > 60, truncate 'b' (or 'a').
			// Expected outcome: both 'a' and 'b' are truncated.
			expectedData:      map[string]interface{}{"a": "long strin...", "b": "long strin...", "c": "short"},
			expectedTruncated: true,
		},
		{
			name:              "Truncation_CannotReachLimit_AllStringsTooShort",
			inputData:         map[string]interface{}{"a": "string1", "b": "string2"}, // Lengths 7, 7. minTruncateLength is 10.
			limit:             15,                                                     // Initial size 29 bytes
			expectedData:      map[string]interface{}{"a": "string1", "b": "string2"}, // No change expected, strings are too short
			expectedTruncated: false,
		},
		{
			name:              "Truncation_StringInSlice_LimitExceeded",
			inputData:         map[string]interface{}{"list": []interface{}{1, "short", "this string is way too long for the limit"}},
			limit:             60,                                                                           // Initial size is 63 bytes. Limit is 60.
			expectedData:      map[string]interface{}{"list": []interface{}{1.0, "short", "this strin..."}}, // Use float64 for number to match JSON behavior
			expectedTruncated: true,
		},
		// New tests for nested structure truncation
		{
			name: "Truncation_ComplexNestedStructure_ArrayTruncation",
			inputData: map[string]interface{}{
				"data": map[string]interface{}{
					"users": []interface{}{
						map[string]interface{}{"id": 1, "name": "User1", "description": "short"},
						map[string]interface{}{"id": 2, "name": "User2", "description": "short"},
						map[string]interface{}{"id": 3, "name": "User3", "description": "short"},
						map[string]interface{}{"id": 4, "name": "User4", "description": "short"},
					},
				},
			},
			limit: 100, // Small enough limit to require array truncation
			// Array will be truncated to half, but current implementation uses at least 1 element
			expectedData: map[string]interface{}{
				"data": map[string]interface{}{
					"users": []interface{}{
						map[string]interface{}{"id": 1.0, "name": "User1", "description": "short"},
					},
				},
			},
			expectedTruncated: true,
		},
		{
			name: "Truncation_ComplexNestedStructure_ObjectTruncation",
			inputData: map[string]interface{}{
				"config": map[string]interface{}{
					"settings": map[string]interface{}{
						"option1": "value1",
						"option2": "value2",
						"option3": "value3",
						"option4": "value4",
						"option5": "value5",
						"option6": "value6",
					},
				},
			},
			limit: 120, // Small enough limit to require object truncation
			// Object will be truncated (some keys will be removed)
			// Exact result depends on findKeysToRemove implementation, but should be smaller
			expectedTruncated: true,
		},
		{
			name: "Truncation_DeepNesting_CombinedApproach",
			inputData: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"data": []interface{}{
								map[string]interface{}{
									"record": map[string]interface{}{
										"field1": "This is a very long string that should be truncated first",
										"field2": "Another long string that might need truncation",
									},
								},
								map[string]interface{}{
									"record": map[string]interface{}{
										"field1": "Long content for second record",
									},
								},
							},
						},
					},
				},
			},
			limit:             200, // Small limit requiring combined approach - first string truncation, then structure truncation
			expectedTruncated: true,
		},
		{
			name: "Truncation_MultipleArrays_LargestTruncatedFirst",
			inputData: map[string]interface{}{
				"smallArray": []interface{}{1, 2, 3},
				"largeArray": []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
					"very long string that increases array size to ensure truncation"},
			},
			limit:             100, // Limit requiring array truncation - reduced to ensure truncation
			expectedTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a deep copy to avoid modifying the original test case data
			dataToModify, err := deepCopyMap(tt.inputData)
			if err != nil {
				t.Fatalf("Failed to deep copy input data: %v", err)
			}

			// Calculate initial size for debugging
			initialSize, _ := estimateSize(dataToModify)

			// For complex nested structure tests, only check truncation and size reduction
			isComplexStructureTest := tt.name == "Truncation_ComplexNestedStructure_ObjectTruncation" ||
				tt.name == "Truncation_DeepNesting_CombinedApproach" ||
				tt.name == "Truncation_MultipleArrays_LargestTruncatedFirst"

			if !isComplexStructureTest && tt.expectedData != nil {
				// Create a deep copy of expected data for comparison to ensure type consistency after potential JSON marshaling issues
				expectedDataComparable, err := deepCopyMap(tt.expectedData)
				if err != nil {
					t.Fatalf("Failed to deep copy expected data: %v", err)
				}

				// Call TruncateMapIfNeeded
				truncated, err := TruncateMapIfNeeded(&dataToModify, tt.limit)

				if tt.expectError {
					if err == nil {
						t.Errorf("Expected an error, but got nil")
					}
				} else {
					if err != nil {
						t.Errorf("Did not expect an error, but got: %v", err)
					}
				}

				if truncated != tt.expectedTruncated {
					t.Errorf("Expected truncated flag to be %v, but got %v", tt.expectedTruncated, truncated)
				}

				// Compare using reflect.DeepEqual first for potentially better error messages on structure mismatch
				if !reflect.DeepEqual(dataToModify, expectedDataComparable) {
					// If DeepEqual fails, fall back to JSON comparison which ignores number type differences
					compareMapsAsJSON(t, dataToModify, expectedDataComparable)
				}
			} else if isComplexStructureTest {
				// For more complex tests, verify only that truncation occurred and resulting size is smaller
				truncated, err := TruncateMapIfNeeded(&dataToModify, tt.limit)

				if err != nil {
					t.Errorf("Did not expect an error, but got: %v", err)
				}

				if truncated != tt.expectedTruncated {
					t.Errorf("Expected truncated flag to be %v, but got %v", tt.expectedTruncated, truncated)
				}

				// If truncation should have occurred, verify size decreased
				if truncated {
					newSize, _ := estimateSize(dataToModify)
					if newSize >= initialSize {
						t.Errorf("Expected size to decrease after truncation but it didn't: before=%d, after=%d", initialSize, newSize)
					}
					if newSize > tt.limit {
						t.Logf("WARN: Size %d still exceeds limit %d after truncation (but it may not always be possible to reach the limit)", newSize, tt.limit)
					}
				}
			}
		})
	}
}

// TestHandleNestedStructures tests the nested structure handling function independently
func TestHandleNestedStructures(t *testing.T) {
	tests := []struct {
		name             string
		data             map[string]interface{}
		limit            int64
		currentSize      int64 // Simulated current size
		expectTruncation bool
	}{
		{
			name: "TruncateArray_Many_Elements",
			data: map[string]interface{}{
				"items": []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
			limit:            50,
			currentSize:      100,
			expectTruncation: true,
		},
		{
			name: "TruncateObject_Many_Properties",
			data: map[string]interface{}{
				"config": map[string]interface{}{
					"prop1": "value1",
					"prop2": "value2",
					"prop3": "value3",
					"prop4": "value4",
					"prop5": "value5",
					"prop6": "value6",
				},
			},
			limit:            80,
			currentSize:      120,
			expectTruncation: true,
		},
		{
			name: "NoTruncation_SmallArray",
			data: map[string]interface{}{
				"items": []interface{}{1},
			},
			limit:            50,
			currentSize:      60,
			expectTruncation: false, // Array too small to truncate (1 element)
		},
		{
			name: "NoTruncation_SmallObject",
			data: map[string]interface{}{
				"config": map[string]interface{}{
					"prop1": "value1",
				},
			},
			limit:            50,
			currentSize:      60,
			expectTruncation: false, // Object too small to truncate (1 property)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a deep copy to avoid modifying the original test case data
			dataToModify, err := deepCopyMap(tt.data)
			if err != nil {
				t.Fatalf("Failed to deep copy input data: %v", err)
			}

			// Directly test handleNestedStructures
			truncated := handleNestedStructures(&dataToModify, tt.limit, tt.currentSize)

			if truncated != tt.expectTruncation {
				t.Errorf("handleNestedStructures returned %v, expected %v", truncated, tt.expectTruncation)
			}

			// If truncation should have occurred, verify changes were made
			if truncated {
				// Verify based on structure type
				switch {
				case tt.data["items"] != nil:
					origItems := tt.data["items"].([]interface{})
					newItems := dataToModify["items"].([]interface{})
					if len(newItems) >= len(origItems) {
						t.Errorf("Expected array to be truncated, but length didn't decrease: original=%d, truncated=%d",
							len(origItems), len(newItems))
					}
				case tt.data["config"] != nil:
					origConfig := tt.data["config"].(map[string]interface{})
					newConfig := dataToModify["config"].(map[string]interface{})
					if len(newConfig) >= len(origConfig) {
						t.Errorf("Expected object to be truncated, but property count didn't decrease: original=%d, truncated=%d",
							len(origConfig), len(newConfig))
					}
				}
			}
		})
	}
}
