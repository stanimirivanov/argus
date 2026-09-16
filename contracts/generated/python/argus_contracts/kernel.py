"""Validation entry point for the generated kernel/v1 binding."""

from .kernel_v1 import KernelFixture


def decode_kernel(data: bytes | str) -> KernelFixture:
    """Decode a kernel document and enforce cross-field identity invariants."""
    document = KernelFixture.model_validate_json(data)
    repository_id = document.repository.id
    references = {
        "/revision/repositoryId": document.revision.repository_id,
        "/component/repositoryId": document.component.repository_id,
        "/evidence/provenance/repositoryId": document.evidence.provenance.repository_id,
    }
    for path, value in references.items():
        if value != repository_id:
            raise ValueError(f"{path}: repository identity mismatch")
    if document.test.suite_id != document.suite.id:
        raise ValueError("/test/suiteId: suite identity mismatch")
    if document.evidence.provenance.revision != document.revision.value:
        raise ValueError("/evidence/provenance/revision: revision mismatch")
    expires_at = document.evidence.expires_at
    if expires_at is not None and expires_at <= document.evidence.observed_at:
        raise ValueError("/evidence/expiresAt: expiry must follow observation")
    if document.compatibility.bundle_version != document.contracts_version:
        raise ValueError("/compatibility/bundleVersion: contracts version mismatch")
    return document
