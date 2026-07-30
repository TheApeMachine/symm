/**
 * GLSL shaders for GPU-accelerated graph visualization
 *
 * EXACT PORT from analytics-master/shaders/
 */

// Passthrough vertex shader - used for GPGPU simulation passes
export const passthruVertexShader = /* glsl */ `
varying vec2 vUv;
void main() {
    vUv = uv;
    gl_Position = vec4(position, 1.0);
}
`;

// Passthrough fragment shader - copies texture data
// Matches fs-passthru.glsl - uses gl_FragCoord and NODESWIDTH for UV calculation
export const passthruFragmentShader = /* glsl */ `
uniform sampler2D uTexture;

const float nodesTexWidth = NODESWIDTH;

void main() {
    vec2 nodeRef = vec2(nodesTexWidth, nodesTexWidth);
    vec2 uv = gl_FragCoord.xy / nodeRef.xy;
    vec4 color = texture2D(uTexture, uv);
    gl_FragColor = color;
}
`;

// Node vertex shader - positions nodes from texture data
// Matches vs-node.glsl
export const nodeVertexShader = /* glsl */ `
attribute vec3 customColor;
attribute vec2 texPos;
attribute float pickingNode;
attribute float threat;

uniform sampler2D positionTexture;
uniform sampler2D nodeAttribTexture;
uniform sampler2D nodeIntensityTexture;
uniform float currentTime;

varying vec3 vColor;
varying float vOpacity;
varying float vIntensity;
varying float vPickingNode;
varying float vThreat;

void main() {
    float nodeSize = 150.0;

    vPickingNode = pickingNode;
    vColor = customColor;
    vThreat = threat;

    vec4 selfPosition = texture2D(positionTexture, texPos);
    vec4 selfAttrib = texture2D(nodeAttribTexture, texPos);
    float intensity = texture2D(nodeIntensityTexture, texPos).r;

    // Boost contrast so "lighting up" reads clearly.
    vIntensity = pow(clamp(intensity, 0.0, 1.0), 0.35);
    vOpacity = clamp(selfAttrib.y + vIntensity * 0.9, 0.0, 1.0);

    if (threat > 0.0) {
        float newSize = sin(currentTime * 0.0025) * nodeSize;
        nodeSize += newSize + nodeSize;
    }

    vec4 mvPosition = modelViewMatrix * vec4(selfPosition.xyz, 1.0);
    float sizeBoost = 1.0 + vIntensity * 1.5;
    gl_PointSize = selfAttrib.x * sizeBoost * (nodeSize / length(mvPosition.xyz));
    gl_Position = projectionMatrix * mvPosition;
}
`;

// Node fragment shader - renders node sprites
// Matches fs-node.glsl
export const nodeFragmentShader = /* glsl */ `
uniform sampler2D sprite;
uniform sampler2D threatSprite;

varying vec3 vColor;
varying float vOpacity;
varying float vIntensity;
varying float vPickingNode;
varying float vThreat;

void main() {
    vec4 node;
    vec3 nodeColor = vColor;

    if (vThreat > 0.0) {
        node = texture2D(threatSprite, vec2(gl_PointCoord.x, gl_PointCoord.y));
    } else {
        node = texture2D(sprite, vec2(gl_PointCoord.x, gl_PointCoord.y));
    }

    // Add a subtle "hot" glow by biasing color toward white as intensity rises.
    nodeColor = mix(nodeColor, vec3(1.0, 1.0, 1.0), vIntensity * 0.6);

    if (vPickingNode > 0.0) {
        gl_FragColor = node * vec4(nodeColor, vOpacity);
    } else {
        gl_FragColor = vec4(nodeColor, 1.0);
    }
}
`;

// Picking fragment shader - outputs solid ID colors for GPU picking
// Similar to fs-node.glsl but no transparency
export const pickingFragmentShader = /* glsl */ `
varying vec3 vColor;
varying float vOpacity;
varying float vPickingNode;
varying float vThreat;

void main() {
    // Calculate distance from center for circular picking area
    vec2 center = gl_PointCoord - vec2(0.5);
    float dist = length(center);

    // Discard pixels outside the circle to avoid picking between nodes
    if (dist > 0.5) discard;

    // Output the encoded node ID color with full alpha
    gl_FragColor = vec4(vColor, 1.0);
}
`;

// Edge vertex shader - positions edge endpoints from texture
// Matches vs-edge.glsl
export const edgeVertexShader = /* glsl */ `
attribute vec2 texPos;
attribute vec3 customColor;

uniform sampler2D positionTexture;
uniform sampler2D nodeAttribTexture;
uniform sampler2D nodeIntensityTexture;

varying vec3 vColor;
varying float vOpacity;

void main() {
    vColor = customColor;

    vec3 nodePosition = texture2D(positionTexture, texPos).xyz;
    vec4 selfAttrib = texture2D(nodeAttribTexture, texPos);
    float intensity = texture2D(nodeIntensityTexture, texPos).r;
    float boosted = pow(clamp(intensity, 0.0, 1.0), 0.35);
    vOpacity = clamp(selfAttrib.y + boosted * 0.5, 0.0, 1.0);

    vec4 mvPosition = modelViewMatrix * vec4(nodePosition, 1.0);
    gl_Position = projectionMatrix * mvPosition;
}
`;

// Edge fragment shader - renders edges with transparency
// Matches fs-edge.glsl
export const edgeFragmentShader = /* glsl */ `
varying vec3 vColor;
varying float vOpacity;

void main() {
    gl_FragColor = vec4(vColor, vOpacity);
}
`;

// Velocity simulation shader - computes force-directed forces
// Matches sim-velocity.glsl
export const velocitySimulationShader = /* glsl */ `
uniform float delta;
uniform float k;
uniform float temperature;
uniform float layoutMode;
uniform float layoutStrength;
uniform vec3 layoutMask;
uniform sampler2D positions;
uniform sampler2D layoutPositions;
uniform sampler2D velocities;
uniform sampler2D edgeIndices;
uniform sampler2D edgeData;

const float nodesTexWidth = NODESWIDTH;
const float edgesTexWidth = EDGESWIDTH;

vec3 getNeighbor(float textureIndex) {
    return texture2D(positions, vec2(
        (mod(textureIndex, nodesTexWidth)) / nodesTexWidth,
        (floor(textureIndex / nodesTexWidth)) / nodesTexWidth
    )).xyz;
}

// Repulsion force: fr(x) = (k*k)/x
vec3 addRepulsion(vec3 self, vec3 neighbor) {
    vec3 diff = self - neighbor;
    float x = length(diff);
    float f = (k * k) / x;
    return normalize(diff) * f;
}

// Attraction force: fa(x) = (x*x)/k
vec3 addAttraction(vec3 self, vec3 neighbor) {
    vec3 diff = self - neighbor;
    float x = length(diff);
    float f = (x * x) / k;
    return normalize(diff) * f;
}

void main() {
    vec2 nodeRef = vec2(nodesTexWidth, nodesTexWidth);
    vec2 uv = gl_FragCoord.xy / nodeRef.xy;
    vec4 selfPosition = texture2D(positions, uv);
    vec4 selfLayoutPosition = texture2D(layoutPositions, uv);
    vec3 selfVelocity = texture2D(velocities, uv).xyz;
    vec3 velocity = selfVelocity;

    vec3 nodePosition;
    vec4 compareNodePosition;
    float speedLimit = 250.0;

    if (layoutMode < 0.5 && selfLayoutPosition.w > 0.0) {
        // Node needs to move towards layout destination
        if (selfPosition.w > 0.0) {
            compareNodePosition = selfLayoutPosition;
            if (distance(compareNodePosition.xyz, selfPosition.xyz) > 0.001) {
                velocity -= addAttraction(selfPosition.xyz, compareNodePosition.xyz);
            }
        }
        velocity *= 0.75;
    } else {
        // Force-directed n-body simulation
        if (selfPosition.w > 0.0) {
            // Repulsion from all other nodes
            for (float y = 0.0; y < nodesTexWidth; y++) {
                for (float x = 0.0; x < nodesTexWidth; x++) {
                    vec2 ref = vec2(x + 0.5, y + 0.5) / nodeRef;
                    compareNodePosition = texture2D(positions, ref);

                    if (distance(compareNodePosition.xyz, selfPosition.xyz) > 0.001) {
                        if (compareNodePosition.w != -1.0) {
                            velocity += addRepulsion(selfPosition.xyz, compareNodePosition.xyz);
                        }
                    }
                }
            }

            // Attraction along edges
            vec4 selfEdgeIndices = texture2D(edgeIndices, uv);
            float idx = selfEdgeIndices.x;
            float idy = selfEdgeIndices.y;
            float idz = selfEdgeIndices.z;
            float idw = selfEdgeIndices.w;

            float start = idx * 4.0 + idy;
            float end = idz * 4.0 + idw;

            if (!(idx == idz && idy == idw)) {
                float edgeIndex = 0.0;
                vec2 edgeRef = vec2(edgesTexWidth, edgesTexWidth);

                for (float y = 0.0; y < edgesTexWidth; y++) {
                    for (float x = 0.0; x < edgesTexWidth; x++) {
                        vec2 ref = vec2(x + 0.5, y + 0.5) / edgeRef;
                        vec4 pixel = texture2D(edgeData, ref);

                        if (edgeIndex >= start && edgeIndex < end) {
                            nodePosition = getNeighbor(pixel.x);
                            velocity -= addAttraction(selfPosition.xyz, nodePosition);
                        }
                        edgeIndex++;

                        if (edgeIndex >= start && edgeIndex < end) {
                            nodePosition = getNeighbor(pixel.y);
                            velocity -= addAttraction(selfPosition.xyz, nodePosition);
                        }
                        edgeIndex++;

                        if (edgeIndex >= start && edgeIndex < end) {
                            nodePosition = getNeighbor(pixel.z);
                            velocity -= addAttraction(selfPosition.xyz, nodePosition);
                        }
                        edgeIndex++;

                        if (edgeIndex >= start && edgeIndex < end) {
                            nodePosition = getNeighbor(pixel.w);
                            velocity -= addAttraction(selfPosition.xyz, nodePosition);
                        }
                        edgeIndex++;
                    }
                }
            }
        }

        // Optional: hybrid constraint mode.
        // When layoutMode >= 0.5 and layoutPositions.w > 0, we add a soft spring toward the layout target
        // on the specified axes (layoutMask). This keeps the simulation "edge-aware" while still preserving
        // an interpretable structure (e.g. layers on Y planes).
        if (layoutMode >= 0.5 && selfLayoutPosition.w > 0.0) {
            vec3 diff = (selfLayoutPosition.xyz - selfPosition.xyz) * layoutMask;
            velocity += diff * layoutStrength;
        }

        // Temperature gradually cools down to zero
        velocity = normalize(velocity) * temperature;
    }

    // Speed limits
    if (length(velocity) > speedLimit) {
        velocity = normalize(velocity) * speedLimit;
    }

    velocity *= 0.25;

    gl_FragColor = vec4(velocity, 1.0);
}
`;

// Position simulation shader - integrates velocity
// Matches sim-position.glsl
export const positionSimulationShader = /* glsl */ `
uniform float delta;
uniform sampler2D positions;
uniform sampler2D velocities;

const float nodesTexWidth = NODESWIDTH;

void main() {
    vec2 nodeRef = vec2(nodesTexWidth, nodesTexWidth);
    vec2 uv = gl_FragCoord.xy / nodeRef.xy;

    vec4 selfPosition = texture2D(positions, uv);
    vec3 selfVelocity = texture2D(velocities, uv).xyz;

    gl_FragColor = vec4(selfPosition.xyz + selfVelocity * delta * 50.0, selfPosition.w);
}
`;

// Node attribute simulation shader - updates visual attributes
// Matches sim-nodeAttrib.glsl
export const nodeAttribSimulationShader = /* glsl */ `
uniform sampler2D nodeIDMappings;
uniform sampler2D epochsIndices;
uniform sampler2D epochsData;
uniform sampler2D nodeAttrib;
uniform sampler2D edgeIndices;
uniform sampler2D edgeData;
uniform float delta;
uniform float minTime;
uniform float maxTime;
uniform float selectedNode;
uniform float hoverMode;

const float nodesTexWidth = NODESWIDTH;
const float epochsTexWidth = EPOCHSWIDTH;
const float edgesTexWidth = EDGESWIDTH;

float inBetweenTimes(float epochTime) {
    if (epochTime >= minTime && epochTime <= maxTime) {
        return 1.0;
    }
    return 0.0;
}

float hasSelectedNeighbor(float neighbor) {
    if (neighbor == selectedNode) {
        return 1.0;
    }
    return 0.0;
}

void main() {
    vec2 nodeRef = vec2(nodesTexWidth, nodesTexWidth);
    vec2 epochsRef = vec2(epochsTexWidth, epochsTexWidth);
    vec2 uv = gl_FragCoord.xy / nodeRef.xy;

    vec4 selfAttrib = texture2D(nodeAttrib, uv);
    vec4 selfEpochIndices = texture2D(epochsIndices, uv);

    float idx = selfEpochIndices.x;
    float idy = selfEpochIndices.y;
    float idz = selfEpochIndices.z;
    float idw = selfEpochIndices.w;

    float start = idx * 4.0 + idy;
    float end = idz * 4.0 + idw;

    float epochPixel = 0.0;
    float neighborPixel = 0.0;
    float selfPixel = texture2D(nodeIDMappings, uv).x;

    // Check if this is the selected node
    if (selfPixel == selectedNode) {
        selfPixel = 1.0;
    } else {
        selfPixel = 0.0;
    }

    // Count epochs in time window
    if (!(idx == idz && idy == idw)) {
        float edgeIndex = 0.0;

        for (float y = 0.0; y < epochsTexWidth; y++) {
            for (float x = 0.0; x < epochsTexWidth; x++) {
                vec2 ref = vec2(x + 0.5, y + 0.5) / epochsRef;
                vec4 pixel = texture2D(epochsData, ref);

                if (edgeIndex >= start && edgeIndex < end) {
                    epochPixel += inBetweenTimes(pixel.x);
                }
                edgeIndex++;

                if (edgeIndex >= start && edgeIndex < end) {
                    epochPixel += inBetweenTimes(pixel.y);
                }
                edgeIndex++;

                if (edgeIndex >= start && edgeIndex < end) {
                    epochPixel += inBetweenTimes(pixel.z);
                }
                edgeIndex++;

                if (edgeIndex >= start && edgeIndex < end) {
                    epochPixel += inBetweenTimes(pixel.w);
                }
                edgeIndex++;
            }
        }
    }

    // Neighbor highlighting - check if connected to selected node
    if (selectedNode >= 0.0) {
        vec4 selfEdgeIndices = texture2D(edgeIndices, uv);

        idx = selfEdgeIndices.x;
        idy = selfEdgeIndices.y;
        idz = selfEdgeIndices.z;
        idw = selfEdgeIndices.w;

        start = idx * 4.0 + idy;
        end = idz * 4.0 + idw;

        if (!(idx == idz && idy == idw)) {
            float edgeIndex = 0.0;
            vec2 edgeRef = vec2(edgesTexWidth, edgesTexWidth);

            for (float y = 0.0; y < edgesTexWidth; y++) {
                for (float x = 0.0; x < edgesTexWidth; x++) {
                    vec2 ref = vec2(x + 0.5, y + 0.5) / edgeRef;
                    vec4 pixel = texture2D(edgeData, ref);

                    if (edgeIndex >= start && edgeIndex < end) {
                        neighborPixel += hasSelectedNeighbor(pixel.x);
                    }
                    edgeIndex++;

                    if (edgeIndex >= start && edgeIndex < end) {
                        neighborPixel += hasSelectedNeighbor(pixel.y);
                    }
                    edgeIndex++;

                    if (edgeIndex >= start && edgeIndex < end) {
                        neighborPixel += hasSelectedNeighbor(pixel.z);
                    }
                    edgeIndex++;

                    if (edgeIndex >= start && edgeIndex < end) {
                        neighborPixel += hasSelectedNeighbor(pixel.w);
                    }
                    edgeIndex++;
                }
            }
        }
    }

    // Animation speed - lower = smoother/slower fade
    float fadeSpeed = 1.2;
    float fastFadeSpeed = 2.0;
    
    // selfAttrib.z stores the initial/base brightness for this node (set from metrics)
    // If z is 0, fall back to y as the base (backwards compatibility)
    float initialBrightness = selfAttrib.z > 0.0 ? selfAttrib.z : selfAttrib.y;
    
    // Brightness multipliers for different states
    float highlightMultiplier = 2.2;  // Boost when highlighted
    float dimMultiplier = 0.3;        // Dim when not connected to hovered node
    float selectedMultiplier = 1.4;   // Moderate boost when connected to selected
    
    if (hoverMode > 0.0) {
        // Hover mode (default, no selection locked)
        
        if (selectedNode >= 0.0) {
            // Hovering over a node
            if (neighborPixel > 0.0 || selfPixel > 0.0) {
                // This node is the hovered node or a neighbor - highlight
                float targetBrightness = min(1.0, initialBrightness * highlightMultiplier);
                if (selfAttrib.y < targetBrightness) {
                    selfAttrib.y += delta * fastFadeSpeed;
                    if (selfAttrib.y > targetBrightness) {
                        selfAttrib.y = targetBrightness;
                    }
                }
                
                if (epochPixel > 0.0) {
                    selfAttrib.x = min(selfAttrib.x * 1.5, 600.0);  // Grow proportionally
                    selfAttrib.y = 1.0;    // Full brightness
                }
            } else {
                // Not connected - dim proportionally
                float targetBrightness = initialBrightness * dimMultiplier;
                if (selfAttrib.y > targetBrightness) {
                    selfAttrib.y -= delta * fadeSpeed;
                    if (selfAttrib.y < targetBrightness) {
                        selfAttrib.y = targetBrightness;
                    }
                }
            }
        } else {
            // Not hovering - return to initial brightness
            if (selfAttrib.y > initialBrightness) {
                selfAttrib.y -= delta * fadeSpeed;
                if (selfAttrib.y < initialBrightness) {
                    selfAttrib.y = initialBrightness;
                }
            }
            if (selfAttrib.y < initialBrightness) {
                selfAttrib.y += delta * fadeSpeed;
                if (selfAttrib.y > initialBrightness) {
                    selfAttrib.y = initialBrightness;
                }
            }
            
            // Show time-based nodes with boost
            if (epochPixel > 0.0) {
                selfAttrib.x = min(selfAttrib.x * 1.5, 600.0);
                selfAttrib.y = min(1.0, initialBrightness * 2.0);
            }
        }

        // Shrink non-highlighted nodes back toward their initial size
        // (initial size is stored and will vary per node)
        if (selfPixel == 0.0 && neighborPixel == 0.0 && epochPixel == 0.0) {
            // No activity - gently shrink oversized nodes
            if (selfAttrib.x > 400.0) {
                selfAttrib.x -= 2000.0 * delta;
            }
        }
    } else {
        // Selection mode - node is locked/selected
        if (neighborPixel > 0.0 || selfPixel > 0.0) {
            // Connected to selected node - moderate brightness
            float targetBrightness = min(0.8, initialBrightness * selectedMultiplier);
            if (selfAttrib.y < targetBrightness) {
                selfAttrib.y += delta * fadeSpeed;
                if (selfAttrib.y > targetBrightness) {
                    selfAttrib.y = targetBrightness;
                }
            }
            if (selfAttrib.y > targetBrightness) {
                selfAttrib.y -= delta * fadeSpeed;
                if (selfAttrib.y < targetBrightness) {
                    selfAttrib.y = targetBrightness;
                }
            }
            
            if (epochPixel > 0.0) {
                selfAttrib.x = min(selfAttrib.x * 1.5, 600.0);
                selfAttrib.y = 1.0;
            }
        } else {
            // Not connected - fade to invisible
            if (selfAttrib.y > 0.0) {
                selfAttrib.y -= delta * fadeSpeed;
                if (selfAttrib.y < 0.0) {
                    selfAttrib.y = 0.0;
                }
            }
        }

        // Shrink non-connected nodes
        if (selfPixel == 0.0 && neighborPixel == 0.0) {
            if (selfAttrib.x > 150.0) {
                selfAttrib.x -= 2000.0 * delta;
            }
        }
    }

    // Preserve z (initial brightness) for future frames to reference
    gl_FragColor = vec4(selfAttrib.xyz, 0.0);
}
`;

// Text/label vertex shader
// Matches vs-text.glsl
// CRITICAL: Original used NO multiplier for labelPositions
export const textVertexShader = /* glsl */ `
attribute vec3 labelPositions;
attribute vec2 texPos;
attribute vec4 textCoord;
attribute vec3 customColor;

uniform sampler2D positionTexture;
uniform sampler2D nodeAttribTexture;
uniform sampler2D nodeIntensityTexture;

varying vec2 vUv;
varying vec4 vTextCoord;
varying vec3 vColor;
varying float vOpacity;

void main() {
    vColor = customColor;
    vUv = uv;
    vTextCoord = textCoord;

    vec3 selfPosition = texture2D(positionTexture, texPos).xyz;
    vec4 selfAttrib = texture2D(nodeAttribTexture, texPos);
    float intensity = texture2D(nodeIntensityTexture, texPos).r;
    float boosted = pow(clamp(intensity, 0.0, 1.0), 0.35);
    vOpacity = clamp(selfAttrib.y + boosted * 0.9, 0.0, 1.0);

    // Transform to view space, add label offset, then project
    // The label offset is added in view space for billboard effect
    gl_Position = projectionMatrix * (
        modelViewMatrix * vec4(selfPosition, 1.0) +
        vec4(labelPositions.xy, 0.0, 0.0)
    );
}
`;

// Text/label fragment shader
// Matches fs-text.glsl
export const textFragmentShader = /* glsl */ `
uniform sampler2D t_text;

varying vec2 vUv;
varying vec4 vTextCoord;
varying vec3 vColor;
varying float vOpacity;

void main() {
    float x = vTextCoord.x;
    float y = vTextCoord.y;
    float w = vTextCoord.z;
    float h = vTextCoord.w;

    // Calculate texture coordinates - note (1.0 - vUv.y) to flip Y axis
    float xF = x + vUv.x * w;
    float yF = y + (1.0 - vUv.y) * h;
    vec2 sCoord = vec2(xF, yF);

    vec4 diffuse = texture2D(t_text, sCoord);

    // Use the diffuse alpha for visibility, multiply by vOpacity for node highlight state
    float alpha = diffuse.a * vOpacity;

    // If alpha is too low (no text at this pixel), discard
    if (alpha < 0.01) discard;

    // Output the node color with the font texture's alpha
    gl_FragColor = vec4(vColor, alpha);
}
`;
