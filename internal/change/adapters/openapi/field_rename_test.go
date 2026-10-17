package openapi

import (
	"errors"
	"strings"
	"testing"
)

const fieldBase = `openapi: 3.1.0
info: {title: Orders, version: '1'}
paths:
  /orders:
    post:
      operationId: createOrder
      x-argus-capabilities: [create-order]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [customerId]
              properties:
                customerId: {type: string, minLength: 1}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                required: [orderId]
                properties:
                  orderId: {type: string}
`

func TestDetectCompatibleFieldRenameRequiresDeclarationAndWholeOperationEquality(t *testing.T) {
	t.Parallel()
	head := strings.ReplaceAll(fieldBase,
		"customerId: {type: string, minLength: 1}",
		"clientId: {type: string, minLength: 1, x-argus-renamed-from: customerId}")
	head = strings.Replace(head, "required: [customerId]", "required: [clientId]", 1)
	proof, err := DetectCompatibleFieldRename([]byte(fieldBase), []byte(head), "api/orders.yaml", "POST", "/orders")
	if err != nil {
		t.Fatalf("detect request field: %v", err)
	}
	if proof.Direction != "request" || proof.PreviousName != "customerId" || proof.Name != "clientId" ||
		proof.StatusCode != "" || proof.BaseSHA256 == proof.HeadSHA256 {
		t.Fatalf("unexpected proof: %+v", proof)
	}

	response := strings.Replace(fieldBase, "orderId: {type: string}",
		"purchaseId: {type: string, x-argus-renamed-from: orderId}", 1)
	response = strings.Replace(response, "required: [orderId]", "required: [purchaseId]", 1)
	proof, err = DetectCompatibleFieldRename([]byte(fieldBase), []byte(response), "api/orders.yaml", "POST", "/orders")
	if err != nil || proof.Direction != "response" || proof.StatusCode != "200" {
		t.Fatalf("detect response field: %+v, %v", proof, err)
	}

	for name, candidate := range map[string]string{
		"undeclared":           strings.ReplaceAll(head, ", x-argus-renamed-from: customerId", ""),
		"type changed":         strings.ReplaceAll(head, "clientId: {type: string", "clientId: {type: integer"),
		"default changed":      strings.ReplaceAll(head, "minLength: 1, x-argus", "minLength: 1, default: hidden, x-argus"),
		"requiredness changed": strings.ReplaceAll(head, "required: [clientId]", "required: []"),
		"flow changed":         strings.ReplaceAll(head, "operationId: createOrder", "operationId: updateOrder"),
		"other schema changed": strings.ReplaceAll(head, "orderId: {type: string}", "orderId: {type: integer}"),
		"ambiguous":            strings.ReplaceAll(head, "orderId: {type: string}", "purchaseId: {type: string, x-argus-renamed-from: orderId}"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DetectCompatibleFieldRename([]byte(fieldBase), []byte(candidate),
				"api/orders.yaml", "POST", "/orders"); !errors.Is(err, ErrNoCompatibleFieldRename) {
				t.Fatalf("wanted abstention, got %v", err)
			}
		})
	}
}
