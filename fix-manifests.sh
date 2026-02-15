#!/bin/bash
# Fix ClusterAgentRole manifests by removing unsupported fields

FILES=(
    "examples/test-manifests/file-delivery.yaml"
    "examples/test-manifests/kitchen-sink.yaml"
    "examples/test-manifests/policy-override.yaml"
    "examples/test-manifests/multi-tier.yaml"
)

for file in "${FILES[@]}"; do
    echo "Fixing $file..."
    # Create a temporary file for the fixed version
    temp_file=$(mktemp)
    
    # Process the file to remove unsupported fields
    awk '
        BEGIN { in_clusterrole = 0; in_spec = 0; skip_line = 0 }
        /^apiVersion: core.hortator.ai\/v1alpha1/ { if (getline && $0 ~ /^kind: ClusterAgentRole/) in_clusterrole = 1; print prev; print; next }
        in_clusterrole && /^spec:/ { in_spec = 1; print; next }
        in_clusterrole && in_spec && /^  (description|rules|antiPatterns|references):/ { 
            skip_line = 1
            # Skip until next field at same indentation or end of spec
            while (getline && ($0 ~ /^    / || $0 ~ /^$/ || ($0 ~ /^  [^ ]/ && $0 !~ /^  (defaultModel|tools|apiKeyRef|defaultEndpoint|health):/))) {
                if ($0 ~ /^  [^ ]/ && $0 !~ /^    /) break
            }
            if ($0 ~ /^  (defaultModel|tools|apiKeyRef|defaultEndpoint|health):/ || $0 ~ /^---/ || $0 ~ /^apiVersion:/) {
                print
            }
            next
        }
        in_clusterrole && /^---/ { in_clusterrole = 0; in_spec = 0 }
        { print }
        { prev = $0 }
    ' "$file" > "$temp_file"
    
    # Replace the original file
    mv "$temp_file" "$file"
done

echo "All ClusterAgentRole manifests fixed!"