// The same expected numbers as PostithekaKit's MascotMotionTests.swift: change one, change both.
import { describe, expect, it } from "vitest";
import { approach, entranceRemaining, MOTION, pointerTarget, poseAt, sampleTrack } from "./mascot-motion";

const [float, sway] = MOTION.idle;

describe("mascot motion", () => {
  it("floats on the flat mascot's five-second hover", () => {
    expect(sampleTrack(float!, 0)).toBeCloseTo(0, 6);
    expect(sampleTrack(float!, 1.25)).toBeCloseTo(0.015, 6);
    expect(sampleTrack(float!, 2.5)).toBeCloseTo(0.03, 6);
    expect(sampleTrack(float!, 5)).toBeCloseTo(0, 6);
    // Time before the start wraps like any other.
    expect(sampleTrack(float!, -1)).toBeCloseTo(0.010365, 6);
  });

  it("sways slowly either side of its rest", () => {
    expect(sampleTrack(sway!, 0)).toBeCloseTo(-3, 6);
    expect(sampleTrack(sway!, 3)).toBeCloseTo(0, 6);
    expect(sampleTrack(sway!, 7.5)).toBeCloseTo(2.12132, 5);
  });

  it("rises in once and settles", () => {
    expect(entranceRemaining(MOTION, 0)).toBeCloseTo(1, 6);
    expect(entranceRemaining(MOTION, 0.3)).toBeCloseTo(0.125, 6);
    expect(entranceRemaining(MOTION, 0.6)).toBeCloseTo(0, 6);
    expect(entranceRemaining(MOTION, 10)).toBeCloseTo(0, 6);
  });

  it("turns toward the pointer, no further than its limits", () => {
    expect(pointerTarget(MOTION, 1.5, -0.5)).toEqual({ yawDegrees: 12, pitchDegrees: 3 });
    expect(pointerTarget(MOTION, -0.5, 1).yawDegrees).toBeCloseTo(-6, 6);
    expect(pointerTarget(MOTION, -0.5, 1).pitchDegrees).toBeCloseTo(-6, 6);
  });

  it("follows quickly at first and never overshoots", () => {
    expect(approach(0, 1, 0.4, 0.4)).toBeCloseTo(0.632121, 6);
    expect(approach(0, 1, 100, 0.4)).toBeLessThanOrEqual(1);
    expect(approach(0, 1, 0.1, 0)).toBe(1);
  });

  it("adds rest, idle, entrance and look into one pose", () => {
    const pose = poseAt(MOTION, 0.3, { yawDegrees: 0, pitchDegrees: 0 });
    expect(pose.translateY).toBeCloseTo(-0.005197, 6);
    expect(pose.yawDegrees).toBeCloseTo(-14.963065, 6);
    expect(pose.pitchDegrees).toBeCloseTo(0, 6);
    expect(pose.scale).toBeCloseTo(0.995, 6);

    const looking = poseAt(MOTION, 3, { yawDegrees: 12, pitchDegrees: 3 });
    expect(looking.yawDegrees).toBeCloseTo(0, 6);
    expect(looking.pitchDegrees).toBeCloseTo(3, 6);
    expect(looking.scale).toBeCloseTo(1, 6);
  });
});
