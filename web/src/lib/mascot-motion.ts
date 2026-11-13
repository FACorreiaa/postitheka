// How the 3D mascot moves (BRD-001). The numbers are `design/mascot-motion.json`, copied beside
// the scene so the bundle carries them; this module only turns a time into values. It imports
// no renderer: `mascot3d/scene.ts` applies what it returns, and PostithekaKit's `MascotMotion`
// is the same arithmetic for iOS. A change here is a change there, and both tests hold the same
// expected numbers.
import data from "./mascot3d/mascot-motion.json";

export type Easing = "sine" | "linear";

export interface Keyframe {
  /** Fraction of the track's period, 0 to 1. */
  t: number;
  v: number;
}

export interface Track {
  property: "translateY" | "rotateYDegrees";
  periodSeconds: number;
  easing: Easing;
  keyframes: Keyframe[];
}

export interface Motion {
  rest: { rotateYDegrees: number };
  idle: Track[];
  /** Where the mascot starts, as offsets from rest, and how long it takes to get there. */
  entrance: { durationSeconds: number; translateY: number; scale: number };
  /** The furthest it turns toward the pointer, and how quickly it follows. */
  pointer: { yawDegrees: number; pitchDegrees: number; timeConstantSeconds: number };
}

/** Where the mascot looks, in degrees: yaw toward +X positive, pitch about X. */
export interface Look {
  yawDegrees: number;
  pitchDegrees: number;
}

/** The whole model's transform for one frame. */
export interface Pose {
  translateY: number;
  yawDegrees: number;
  pitchDegrees: number;
  scale: number;
}

export const MOTION = data as Motion;

/** 0 to 1 along one span between two keyframes. */
export function ease(easing: Easing, u: number): number {
  return easing === "sine" ? 0.5 - 0.5 * Math.cos(Math.PI * u) : u;
}

/** A looping track's value at `seconds`. */
export function sampleTrack(track: Track, seconds: number): number {
  const frames = track.keyframes;
  const period = track.periodSeconds;
  const phase = (((seconds % period) + period) % period) / period;
  for (let i = 1; i < frames.length; i++) {
    const from = frames[i - 1]!;
    const to = frames[i]!;
    if (phase <= to.t) {
      const span = to.t - from.t;
      const u = span > 0 ? (phase - from.t) / span : 1;
      return from.v + (to.v - from.v) * ease(track.easing, u);
    }
  }
  return frames[frames.length - 1]?.v ?? 0;
}

/** How much of the entrance is still to go at `seconds`: 1 at the start, 0 once it has
 * settled. Ease-out cubic, so it arrives quickly and lands softly. */
export function entranceRemaining(motion: Motion, seconds: number): number {
  const u = Math.min(1, Math.max(0, seconds / motion.entrance.durationSeconds));
  return (1 - u) ** 3;
}

/** Where to look for a pointer at (`x`, `y`): each from -1 to 1 across the page, right and up
 * positive, held inside that range. Looking up is a negative pitch (a turn about X tips the
 * face down). */
export function pointerTarget(motion: Motion, x: number, y: number): Look {
  const cx = Math.max(-1, Math.min(1, x));
  const cy = Math.max(-1, Math.min(1, y));
  return { yawDegrees: cx * motion.pointer.yawDegrees, pitchDegrees: -cy * motion.pointer.pitchDegrees };
}

/** One frame's step from `current` toward `target`: quick at first, never past it. */
export function approach(current: number, target: number, deltaSeconds: number, timeConstantSeconds: number): number {
  if (timeConstantSeconds <= 0) return target;
  return current + (target - current) * (1 - Math.exp(-deltaSeconds / timeConstantSeconds));
}

/** The pose `seconds` after the mascot appeared, looking at `look`: rest, plus the idle loops,
 * plus what is left of the entrance, plus the look. */
export function poseAt(motion: Motion, seconds: number, look: Look): Pose {
  let translateY = 0;
  let yawDegrees = motion.rest.rotateYDegrees + look.yawDegrees;
  for (const track of motion.idle) {
    const value = sampleTrack(track, seconds);
    if (track.property === "translateY") translateY += value;
    else yawDegrees += value;
  }
  const left = entranceRemaining(motion, seconds);
  return {
    translateY: translateY + motion.entrance.translateY * left,
    yawDegrees,
    pitchDegrees: look.pitchDegrees,
    scale: 1 + (motion.entrance.scale - 1) * left,
  };
}
