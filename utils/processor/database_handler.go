package processor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/database"
)

// handleDatabaseInput processes database input operations
func (p *Processor) handleDatabaseInput(input interface{}) error {
	// Input should be a map with database configuration
	dbInput, ok := input.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid database input format")
	}

	// Extract database name and SQL statement
	dbName, ok := dbInput["database"].(string)
	if !ok {
		return fmt.Errorf("database name not specified")
	}

	sql, ok := dbInput["sql"].(string)
	if !ok {
		return fmt.Errorf("SQL statement not specified")
	}

	// Create database handler
	dbHandler := database.NewHandler(p.envConfig)
	defer dbHandler.Close()

	// Determine operation type based on SQL
	sql = strings.TrimSpace(strings.ToUpper(sql))
	if strings.HasPrefix(sql, "SELECT") {
		// Handle read operation
		results, err := dbHandler.ExecuteRead(dbName, sql)
		if err != nil {
			return fmt.Errorf("database read error: %w", err)
		}

		// Convert results to JSON for storage
		jsonData, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("error converting results to JSON: %w", err)
		}

		p.lastOutput = string(jsonData)
		return nil
	} else {
		// Handle write operation
		affected, err := dbHandler.ExecuteWrite(dbName, sql)
		if err != nil {
			return fmt.Errorf("database write error: %w", err)
		}

		p.lastOutput = fmt.Sprintf("Affected rows: %d", affected)
		return nil
	}
}

// handleDatabaseOutput processes database output operations
func (p *Processor) handleDatabaseOutput(output string, dbConfig map[string]interface{}) error {
	// Extract database name and SQL statement
	dbName, ok := dbConfig["database"].(string)
	if !ok {
		return fmt.Errorf("database name not specified")
	}

	sql, ok := dbConfig["sql"].(string)
	if !ok {
		return fmt.Errorf("SQL statement not specified")
	}

	// Create database handler
	dbHandler := database.NewHandler(p.envConfig)
	defer dbHandler.Close()

	// Validate that this is a write operation
	if err := dbHandler.ValidateOperation(sql, database.WriteOperation); err != nil {
		return fmt.Errorf("invalid database output operation: %w", err)
	}

	// Execute write operation
	affected, err := dbHandler.ExecuteWrite(dbName, sql)
	if err != nil {
		return fmt.Errorf("database write error: %w", err)
	}

	p.lastOutput = fmt.Sprintf("Affected rows: %d", affected)
	return nil
}
