package processor

import (
	"strings"
)

// NormalizeStringSlice converts interface{} to []string
func (p *Processor) NormalizeStringSlice(val interface{}) []string {
	p.debugf("Normalizing value type: %T", val)
	if val == nil {
		p.debugf("Received nil value, returning empty string slice")
		return []string{}
	}

	switch v := val.(type) {
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			switch itemVal := item.(type) {
			case string:
				result[i] = itemVal
			case map[string]interface{}:
				// Handle map with filename key
				if filename, ok := itemVal["filename"].(string); ok {
					result[i] = filename
				}
			case map[interface{}]interface{}:
				// Handle map with filename key (YAML parsing might produce this type)
				if filename, ok := itemVal["filename"].(string); ok {
					result[i] = filename
				}
			}
		}
		p.debugf("Converted []interface{} to []string: %v", result)
		return result
	case []string:
		p.debugf("Value already []string: %v", v)
		return v
	case string:
		p.debugf("Converting single string to []string: %v", v)
		// Check if the string contains a filenames tag with comma-separated values
		if strings.HasPrefix(v, "filenames:") {
			files := strings.TrimPrefix(v, "filenames:")
			// Split by comma and trim spaces
			fileList := strings.Split(files, ",")
			result := make([]string, 0, len(fileList))
			for _, file := range fileList {
				trimmed := strings.TrimSpace(file)
				if trimmed != "" {
					result = append(result, trimmed)
				}
			}
			p.debugf("Parsed filenames tag into []string: %v", result)
			return result
		}
		return []string{v}
	case map[string]interface{}:
		// Handle single map with filename key
		if filename, ok := v["filename"].(string); ok {
			return []string{filename}
		}
		return []string{}
	case map[interface{}]interface{}:
		// Handle single map with filename key (YAML parsing might produce this type)
		if filename, ok := v["filename"].(string); ok {
			return []string{filename}
		}
		return []string{}
	default:
		p.debugf("Unsupported type, returning empty string slice")
		return []string{}
	}
}
