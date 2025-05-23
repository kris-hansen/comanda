# Adding a New Model to COMandA

This guide outlines the steps for adding support for a new Large Language Model (LLM) to COMandA, assuming the provider (e.g., Anthropic, OpenAI) is already integrated.

## Prerequisites

*   A working COMandA development environment.
*   Basic knowledge of Go programming.
*   An API key (if required) for the new model.

## Steps

1.  **Update the Model List in `cmd/configure.go`:**

    *   Locate the provider's model list function. These functions are typically named `get<ProviderName>Models()` (e.g., `getAnthropicModels()`, `getOpenAIModels()`).
    *   Add the new model's name to the string slice returned by this function. This will make the model selectable in the `comanda configure` wizard.

    Example (adding a new Anthropic model):

    ```go
    func getAnthropicModels() []string {
        return []string{
            "claude-3-opus-20240514",
            "claude-3-sonnet-20240514",
            // Add the new model here:
            "claude-new-model-20250101",
        }
    }
    ```

2.  **Update the Provider's Validation Logic in `utils/models/<provider_name>.go`:**

    *   Find the provider's implementation in the `utils/models/` directory (e.g., `anthropic.go`, `openai.go`).
    *   Locate the `ValidateModel` function. This function determines whether a given model name is valid for that provider.
    *   Add the new model to the validation logic. This usually involves adding the model name to a list of allowed models or creating a pattern that matches the new model.

    Example (adding a new Anthropic model):

    ```go
    func (a *AnthropicProvider) ValidateModel(modelName string) bool {
        validModels := []string{
            "claude-3-opus-20240514",
            "claude-3-sonnet-20240514",
            // Add the new model here:
            "claude-new-model-20250101",
        }

        // ... other validation logic ...
    }
    ```

3.  **Update the Main `README.md` (Optional):**

    *   In the "Provider Configuration" section, add the new model to the list of configured models for the relevant provider.

4.  **Create a New Example YAML File (Recommended):**

    *   Create a new YAML file in the `examples/model-examples/` directory that demonstrates how to use the new model in a COMandA workflow.
    *   This helps users understand how to use the model and provides a working example for testing.

5.  **Inform Users to Run `comanda configure`:**

    *   After making these code changes, users updating an existing COMandA installation will need to run `comanda configure`, select the relevant provider, and then select the new model to enable it in their local configuration.

## Testing

*   After making these changes, run `go test ./...` to ensure that all tests pass.
*   Test the new model by running COMandA with a YAML file that uses it.

## Contributing

*   When submitting a pull request, please ensure that your changes follow the existing code style and include tests for any new functionality.

By following these steps, you can easily add support for new models to existing providers in COMandA.
