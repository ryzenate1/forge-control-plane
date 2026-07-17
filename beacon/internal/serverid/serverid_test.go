package serverid

import "testing"

func TestValidate(t *testing.T) {
	valid := []string{
		"123e4567-e89b-12d3-a456-426614174000",
		"123E4567-E89B-12D3-A456-426614174000",
	}
	for _, value := range valid {
		if err := Validate(value); err != nil {
			t.Fatalf("expected %q to be valid: %v", value, err)
		}
	}

	invalid := []string{
		"",
		"demo",
		"../123e4567-e89b-12d3-a456-426614174000",
		"123e4567/e89b/12d3/a456/426614174000",
		"123e4567-e89b-12d3-a456-42661417400z",
		"123e4567e89b12d3a456426614174000",
	}
	for _, value := range invalid {
		if err := Validate(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}
