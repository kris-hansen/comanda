# Adding a New Model to COMandA

This guide outlines the steps for adding support for a new Large Language Model (LLM) to COMandA, assuming the provider (e.g., Anthropic, OpenAI) is already integrated.

## Prerequisites

*   A working COMandA development environment.
*   Basic knowledge of Go programming.
*   An API key (if required) for the new model.

## Steps

1.  **Update the Central Model Registry in `utils/models/registry.go`:**

    *   Locate the `initializeDefaultModels` method in the `ModelRegistry` struct.
    *   Add the new model's name to the appropriate provider's model list using the `RegisterModels` method.
    *   If the model belongs to a new model family (prefix), add it to the provider's family list using the `RegisterFamilies` method.

    Example (adding a new Anthropic model):

    ```go
    // In utils/models/registry.go, inside initializeDefaultModels method
    r.RegisterModels("anthropic", []string{
        "claude-3-5-sonnet-20241022",
        "claude-3-5-sonnet-latest",
        "claude-3-5-haiku-latest",
        "claude-3-7-sonnet-20250219",
        "claude-3-7-sonnet-latest",
        "claude-3-5-haiku-20241022",
        "claude-opus-4-20250514",
        "claude-sonnet-4-20250514",
        // Add the new model here:
        "claude-new-model-20250101",
    })
    
    // If the model belongs to a new family, add it here:
    r.RegisterFamilies("anthropic", []string{
        "claude-3-5-sonnet",
        "claude-3-5-haiku",
        "claude-3-7-sonnet",
        "claude-opus-4",
        "claude-sonnet-4",
        // Add the new family here if needed:
        "claude-new-family",
    })
    ```

2.  **Update the Main `README.md` (Optional):**

    *   In the "Provider Configuration" section, add the new model to the list of configured models for the relevant provider.

3.  **Create a New Example YAML File (Recommended):**

    *   Create a new YAML file in the `examples/model-examples/` directory that demonstrates how to use the new model in a COMandA workflow.
    *   This helps users understand how to use the model and provides a working example for testing.

4.  **Inform Users to Run `comanda configure`:**

    *   After making these code changes, users updating an existing COMandA installation will need to run `comanda configure`, select the relevant provider, and then select the new model to enable it in their local configuration.

## Testing

*   After making these changes, run `go test ./...` to ensure that all tests pass.
*   Test the new model by running COMandA with a YAML file that uses it.

## Contributing

*   When submitting a pull request, please ensure that your changes follow the existing code style and include tests for any new functionality.

By following these steps, you can easily add support for new models to existing providers in COMandA.
