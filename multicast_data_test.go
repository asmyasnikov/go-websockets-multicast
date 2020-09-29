package multicast

import (
	"fmt"
)

type TestData struct {
	Field1 *int    `json:"field_1,omitempty"`
	Field2 *string `json:"field_2,omitempty"`
}

type TestDataMarshal struct {
	Field1 int `json:"field_1"`
	Field2 int `json:"field_2"`
}

func (t TestDataMarshal) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"field_1":%d,"field_2":%d}`, t.Field1, t.Field2)), nil
}

func NewTestData(field1 *int, field2 *string) *TestData {
	return &TestData{Field1: field1, Field2: field2}
}

func NewEmptyData() *TestData {
	return &TestData{}
}
