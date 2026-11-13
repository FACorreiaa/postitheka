// The 3D mascot's renderer (BRD-001). The only module that imports `three`: `Mascot3D.tsx`
// loads it with a dynamic import after first paint, so it is its own chunk and never downloads
// for someone who does not see the mascot.
import {
  ACESFilmicToneMapping,
  Color,
  DirectionalLight,
  Group,
  MathUtils,
  PerspectiveCamera,
  PMREMGenerator,
  Scene,
  WebGLRenderer,
} from "three";
import type { Material, Mesh, Object3D } from "three";
import { RoomEnvironment } from "three/addons/environments/RoomEnvironment.js";
import { GLTFLoader } from "three/addons/loaders/GLTFLoader.js";
import { approach, type Look, MOTION, pointerTarget, poseAt } from "../mascot-motion";

interface MascotScene {
  /** Where to look: each from -1 to 1 across the page, right and up positive. */
  pointAt: (x: number, y: number) => void;
  /** Draw or rest: off-screen and in a hidden tab the loop stops. */
  setRunning: (running: boolean) => void;
  /** Match the canvas to its box. */
  resize: () => void;
  dispose: () => void;
}

// build_mascot3d.py makes the model one unit tall, standing on the origin, facing +Z.
const FIELD_OF_VIEW = 30;
const CAMERA = { y: 0.55, z: 2.4, aimY: 0.5 };
const MAX_PIXEL_RATIO = 2;
/** A frame that took longer (a tab coming back) moves the mascot this far and no further. */
const MAX_STEP_SECONDS = 0.1;

function isMesh(node: Object3D): node is Mesh {
  return (node as Mesh).isMesh === true;
}

/** A colour token from `styles.css`, so the lights stay in the palette. */
function token(name: string): Color {
  return new Color(getComputedStyle(document.documentElement).getPropertyValue(name).trim());
}

/** Loads the model into `canvas`, inside `host`, and starts the mascot's entrance. It draws
 * only while `host` is on screen and the tab is visible, and turns toward the pointer. Rejects
 * when WebGL is missing or the model cannot be read; the caller keeps the flat mascot.
 * `onFirstFrame` runs once the first frame is on the canvas. The returned function stops
 * everything and frees the context.
 *
 * The page wiring (observers, listeners) lives here rather than in `Mascot3D.tsx` so that it
 * downloads with three, not with the entry every visitor loads. */
export async function mountMascot(
  host: HTMLElement,
  canvas: HTMLCanvasElement,
  options: { modelUrl: string; onFirstFrame: () => void },
): Promise<() => void> {
  const scene = await loadScene(canvas, options);
  let onScreen = false;
  const run = () => scene.setRunning(onScreen && !document.hidden);
  const follow = (event: PointerEvent) => {
    const rect = host.getBoundingClientRect();
    // From the mascot's middle to the edge of the page is the whole of its turn.
    scene.pointAt(
      (event.clientX - (rect.left + rect.width / 2)) / (window.innerWidth / 2),
      (rect.top + rect.height / 2 - event.clientY) / (window.innerHeight / 2),
    );
  };
  const watcher = new IntersectionObserver(([entry]) => {
    onScreen = entry?.isIntersecting ?? false;
    run();
  });
  const sizer = new ResizeObserver(() => scene.resize());
  watcher.observe(host);
  sizer.observe(host);
  document.addEventListener("visibilitychange", run);
  window.addEventListener("pointermove", follow, { passive: true });
  return () => {
    watcher.disconnect();
    sizer.disconnect();
    document.removeEventListener("visibilitychange", run);
    window.removeEventListener("pointermove", follow);
    scene.dispose();
  };
}

async function loadScene(
  canvas: HTMLCanvasElement,
  options: { modelUrl: string; onFirstFrame: () => void },
): Promise<MascotScene> {
  const renderer = new WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: "low-power" });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, MAX_PIXEL_RATIO));
  renderer.toneMapping = ACESFilmicToneMapping;

  const scene = new Scene();
  // Something for the metal to reflect, without an HDR file to download: unlit, a metallic
  // gold renders nearly black.
  const pmrem = new PMREMGenerator(renderer);
  const room = new RoomEnvironment();
  const environment = pmrem.fromScene(room, 0.04);
  scene.environment = environment.texture;
  scene.environmentIntensity = 0.6;
  room.dispose();
  pmrem.dispose();

  // The key art's light: gold from the front, the hologram's cyan on the edge.
  const key = new DirectionalLight(token("--gold"), 2.4);
  key.position.set(1.4, 1.8, 2.2);
  const rim = new DirectionalLight(token("--postmark"), 1.6);
  rim.position.set(-1.6, 1.2, -2);
  scene.add(key, rim);

  const camera = new PerspectiveCamera(FIELD_OF_VIEW, 1, 0.1, 20);
  camera.position.set(0, CAMERA.y, CAMERA.z);
  camera.lookAt(0, CAMERA.aimY, 0);

  let model: Object3D;
  try {
    model = (await new GLTFLoader().loadAsync(options.modelUrl)).scene;
  } catch (error) {
    environment.dispose();
    renderer.dispose();
    renderer.forceContextLoss();
    throw error;
  }
  // The model has no rig: everything moves it as one, through this pivot.
  const pivot = new Group();
  pivot.add(model);
  // Yaw, then pitch: the order MascotMotion.swift composes them in.
  pivot.rotation.order = "YXZ";
  scene.add(pivot);

  const geometries = new Set<{ dispose: () => void }>();
  const materials = new Set<Material>();
  model.traverse((node) => {
    if (!isMesh(node)) return;
    geometries.add(node.geometry);
    for (const material of [node.material].flat()) materials.add(material);
  });

  const look: Look = { yawDegrees: 0, pitchDegrees: 0 };
  let target: Look = { yawDegrees: 0, pitchDegrees: 0 };
  let seconds = 0;

  function pose(delta: number) {
    seconds += delta;
    const tau = MOTION.pointer.timeConstantSeconds;
    look.yawDegrees = approach(look.yawDegrees, target.yawDegrees, delta, tau);
    look.pitchDegrees = approach(look.pitchDegrees, target.pitchDegrees, delta, tau);
    const now = poseAt(MOTION, seconds, look);
    pivot.position.y = now.translateY;
    pivot.rotation.set(MathUtils.degToRad(now.pitchDegrees), MathUtils.degToRad(now.yawDegrees), 0);
    pivot.scale.setScalar(now.scale);
  }

  function resize() {
    const width = canvas.clientWidth;
    const height = canvas.clientHeight;
    if (width === 0 || height === 0) return;
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    camera.updateProjectionMatrix();
  }

  let request = 0;
  let last = 0;
  let running = false;
  let drawn = false;
  let disposed = false;

  function draw(now: number) {
    request = 0;
    if (!running || disposed) return;
    const delta = last ? Math.min((now - last) / 1000, MAX_STEP_SECONDS) : 0;
    last = now;
    pose(delta);
    renderer.render(scene, camera);
    if (!drawn) {
      drawn = true;
      options.onFirstFrame();
    }
    request = requestAnimationFrame(draw);
  }

  resize();
  pose(0);

  return {
    pointAt(x, y) {
      target = pointerTarget(MOTION, x, y);
    },
    setRunning(next) {
      if (disposed || next === running) return;
      running = next;
      // The time away is not played back: the mascot carries on from where it stopped.
      last = 0;
      if (running && !request) request = requestAnimationFrame(draw);
    },
    resize,
    dispose() {
      disposed = true;
      running = false;
      if (request) cancelAnimationFrame(request);
      for (const geometry of geometries) geometry.dispose();
      for (const material of materials) material.dispose();
      environment.dispose();
      renderer.dispose();
      // Free the context now rather than at garbage collection, so a remount (React's
      // StrictMode, or coming back to sign-in) never holds two.
      renderer.forceContextLoss();
    },
  };
}
