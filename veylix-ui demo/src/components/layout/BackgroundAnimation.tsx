import React, { useEffect, useRef } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';

export const BackgroundAnimation: React.FC = () => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const { themeConfig } = useVeylixStore();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let animationFrameId: number;
    let width = (canvas.width = window.innerWidth);
    let height = (canvas.height = window.innerHeight);

    const handleResize = () => {
      if (!canvas) return;
      width = canvas.width = window.innerWidth;
      height = canvas.height = window.innerHeight;
    };
    window.addEventListener('resize', handleResize);

    // Mouse coordinates for interactive attraction
    const mouse = { x: width / 2, y: height / 2, active: false };
    const handleMouseMove = (e: MouseEvent) => {
      mouse.x = e.clientX;
      mouse.y = e.clientY;
      mouse.active = true;
    };
    const handleMouseLeave = () => {
      mouse.active = false;
    };
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseleave', handleMouseLeave);

    const primaryColor = themeConfig.primaryColor || '#10b981';
    const accentColor = themeConfig.accentColor || '#059669';
    const preset = themeConfig.preset || 'modern';
    const bgAnimType = themeConfig.bgAnimationType;

    // Determine active animation mode: strictly matched to theme if not explicitly chosen
    let currentMode = 'particles';
    if (bgAnimType && bgAnimType !== 'none') {
      currentMode = bgAnimType;
    } else if (preset === 'gaming') {
      currentMode = 'cyber_grid';
    } else if (preset === 'classic') {
      currentMode = 'scanline_crt';
    } else if (preset === 'dark') {
      currentMode = 'subtle_orbs';
    } else if (preset === 'light') {
      currentMode = 'aurora_wave';
    } else {
      currentMode = 'particles';
    }

    // -------------------------------------------------------------
    // DATA INITIALIZATION FOR ALL THEME ANIMATION MODES
    // -------------------------------------------------------------

    // 1. PARTICLES / CONSTELLATION (Pro)
    const nodeCount = Math.min(width < 768 ? 35 : 65, 80);
    const nodes: Array<{
      x: number;
      y: number;
      vx: number;
      vy: number;
      radius: number;
      color: string;
      pulse: number;
    }> = [];
    for (let i = 0; i < nodeCount; i++) {
      nodes.push({
        x: Math.random() * width,
        y: Math.random() * height,
        vx: (Math.random() - 0.5) * 0.85,
        vy: (Math.random() - 0.5) * 0.85,
        radius: Math.random() * 2.5 + 1.5,
        color: i % 3 === 0 ? primaryColor : (i % 3 === 1 ? accentColor : '#38bdf8'),
        pulse: Math.random() * Math.PI * 2
      });
    }
    const networkPackets: Array<{
      fromIdx: number;
      toIdx: number;
      progress: number;
      speed: number;
      color: string;
    }> = [];
    for (let i = 0; i < 16; i++) {
      networkPackets.push({
        fromIdx: Math.floor(Math.random() * nodeCount),
        toIdx: Math.floor(Math.random() * nodeCount),
        progress: Math.random(),
        speed: 0.008 + Math.random() * 0.012,
        color: i % 2 === 0 ? '#34d399' : '#38bdf8'
      });
    }

    // 2. CYBER GRID (Gaming)
    let gridScroll = 0;
    const cyberPackets: Array<{
      xRatio: number;
      progress: number;
      speed: number;
      length: number;
      color: string;
    }> = [];
    for (let i = 0; i < 22; i++) {
      cyberPackets.push({
        xRatio: (Math.floor(Math.random() * 21) - 10) / 10,
        progress: Math.random(),
        speed: 0.007 + Math.random() * 0.012,
        length: 0.08 + Math.random() * 0.12,
        color: i % 3 === 0 ? primaryColor : (i % 3 === 1 ? '#06b6d4' : '#f43f5e')
      });
    }

    // 3. GAMING WARP (Gaming Hyperspace)
    const warpStars: Array<{
      x: number;
      y: number;
      z: number;
      pz: number;
      color: string;
    }> = [];
    const warpCount = width < 768 ? 90 : 180;
    for (let i = 0; i < warpCount; i++) {
      warpStars.push({
        x: (Math.random() - 0.5) * width * 2,
        y: (Math.random() - 0.5) * height * 2,
        z: Math.random() * width,
        pz: width,
        color: i % 3 === 0 ? '#06b6d4' : (i % 3 === 1 ? '#f43f5e' : '#a855f7')
      });
    }

    // 4. GAMING HEX GRID (Gaming)
    let hexPulse = 0;

    // 5. MATRIX CODE RAIN (Classic)
    const matrixChars = '0123456789ABCDEF01アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホ';
    const charSize = width < 768 ? 13 : 16;
    const columns = Math.floor(width / charSize) + 1;
    const drops: Array<{ y: number; speed: number; chars: string[] }> = [];
    for (let i = 0; i < columns; i++) {
      drops[i] = {
        y: Math.random() * -100,
        speed: 0.8 + Math.random() * 1.6,
        chars: Array.from({ length: 25 }, () => matrixChars[Math.floor(Math.random() * matrixChars.length)])
      };
    }

    // 6. CLASSIC OSCILLOSCOPE (Classic)
    let oscPhase = 0;

    // 7. CLASSIC TERMINAL BOOT SHELL (Classic)
    const terminalLogs = [
      'kernel: [0.000000] Linux version 6.8.9-veylix-x86_64 (root@vpn-cluster-sg1)',
      'systemd[1]: Detected architecture x86-64.',
      '3x-ui.service: Initialized SQLite WAL encrypted storage at /etc/x-ui/x-ui.db',
      'veylix-core: Inbound [VLESS-Reality:443] listening on 0.0.0.0/0 (xtls-rprx-vision)',
      'veylix-core: Inbound [VMess-gRPC:8443] TLS handshake verified via Cloudflare SNI',
      'veylix-core: Inbound [Trojan-gRPC:2096] cipher 2022-blake3-aes-256-gcm active',
      'cluster-ping: Singapore srv_sg_01 latency probe: 24ms (loss: 0.0%)',
      'cluster-ping: Frankfurt srv_fra_01 latency probe: 88ms (loss: 0.0%)',
      'cluster-ping: Tokyo srv_tyo_02 latency probe: 42ms (loss: 0.0%)',
      'cluster-ping: New York srv_us_03 latency probe: 112ms (loss: 0.0%)',
      'netfilter: IPTABLES masquerade rules committed. Forwarding enabled: net.ipv4.ip_forward=1',
      'veylix-node: Memory allocator Argon2id: 64MB memory cost, 2 parallel lanes',
      'systemd[1]: Started Veylix VPN Master Gateway Daemon v1.8.28.'
    ];

    // 8. WORLD MAP PINGS (Pro)
    const worldHubs = [
      { name: 'Singapore', x: 0.76, y: 0.62, ping: '24ms', color: '#10b981' },
      { name: 'Tokyo', x: 0.84, y: 0.44, ping: '42ms', color: '#06b6d4' },
      { name: 'Frankfurt', x: 0.51, y: 0.35, ping: '88ms', color: '#3b82f6' },
      { name: 'New York', x: 0.28, y: 0.38, ping: '112ms', color: '#8b5cf6' },
      { name: 'Sydney', x: 0.88, y: 0.78, ping: '130ms', color: '#f59e0b' }
    ];

    // 9. PRO FIBER OPTICS (Pro)
    const fiberBuses: Array<{ y: number; speed: number; pulses: number[]; color: string }> = [];
    for (let i = 0; i < 9; i++) {
      fiberBuses.push({
        y: (i + 1) * (height / 10),
        speed: 2 + Math.random() * 3,
        pulses: [Math.random() * width, Math.random() * width, Math.random() * width],
        color: i % 2 === 0 ? primaryColor : '#06b6d4'
      });
    }

    // 10. COSMIC NEBULA & STARS (Dark)
    const starCount = width < 768 ? 70 : 130;
    const stars: Array<{
      x: number;
      y: number;
      size: number;
      alpha: number;
      pulseSpeed: number;
      hasCrossGlow: boolean;
    }> = [];
    for (let i = 0; i < starCount; i++) {
      stars.push({
        x: Math.random() * width,
        y: Math.random() * height,
        size: Math.random() * 2.2 + 0.8,
        alpha: Math.random() * 0.7 + 0.3,
        pulseSpeed: 0.015 + Math.random() * 0.03,
        hasCrossGlow: Math.random() > 0.82
      });
    }
    let meteor: { x: number; y: number; length: number; speed: number; alpha: number } | null = null;
    let nextMeteorTimer = 60;

    // 11. DARK STEALTH RADAR (Dark)
    let radarAngle = 0;
    const stealthBlips: Array<{ angle: number; dist: number; alpha: number; label: string }> = [
      { angle: 0.4, dist: 0.4, alpha: 0, label: 'Node-SG' },
      { angle: 1.8, dist: 0.65, alpha: 0, label: 'Node-FRA' },
      { angle: 3.2, dist: 0.3, alpha: 0, label: 'Node-TYO' },
      { angle: 4.6, dist: 0.8, alpha: 0, label: 'Node-NYC' },
      { angle: 5.7, dist: 0.55, alpha: 0, label: 'Node-SYD' }
    ];

    // 12. AURORA WAVES & BOKEH (Light)
    let auroraPhase = 0;
    const bokehOrbs: Array<{
      x: number;
      y: number;
      radius: number;
      vx: number;
      vy: number;
      color: string;
      alpha: number;
    }> = [];
    for (let i = 0; i < 18; i++) {
      bokehOrbs.push({
        x: Math.random() * width,
        y: Math.random() * height,
        radius: 60 + Math.random() * 120,
        vx: (Math.random() - 0.5) * 0.4,
        vy: (Math.random() - 0.5) * 0.4,
        color: i % 2 === 0 ? primaryColor : (i % 3 === 0 ? '#38bdf8' : '#8b5cf6'),
        alpha: 0.08 + Math.random() * 0.12
      });
    }

    // 13. LIGHT PRISMATIC CAUSTICS (Light)
    let causticsTime = 0;

    // 14. LIGHT MINIMAL GEOMETRIC (Light)
    const geometricShapes: Array<{
      x: number;
      y: number;
      size: number;
      rot: number;
      vrot: number;
      vx: number;
      vy: number;
      type: 'cube' | 'tetra' | 'octa';
    }> = [];
    for (let i = 0; i < 10; i++) {
      geometricShapes.push({
        x: Math.random() * width,
        y: Math.random() * height,
        size: 35 + Math.random() * 45,
        rot: Math.random() * Math.PI * 2,
        vrot: (Math.random() - 0.5) * 0.015,
        vx: (Math.random() - 0.5) * 0.35,
        vy: (Math.random() - 0.5) * 0.35,
        type: i % 3 === 0 ? 'cube' : (i % 3 === 1 ? 'tetra' : 'octa')
      });
    }

    let tick = 0;

    // -------------------------------------------------------------
    // MAIN RENDER LOOP (60 FPS)
    // -------------------------------------------------------------
    const render = () => {
      tick++;

      // When animations disabled or set to none
      if (!themeConfig.enableAnimations || currentMode === 'none') {
        ctx.clearRect(0, 0, width, height);
        animationFrameId = requestAnimationFrame(render);
        return;
      }

      // ===========================================================
      // [GAMING CATEGORY ANIMATIONS]
      // ===========================================================

      // MODE: CYBER GRID (Gaming HUD Tron Horizon)
      if (currentMode === 'cyber_grid') {
        ctx.clearRect(0, 0, width, height);
        gridScroll = (gridScroll + 1.4) % 50;
        const horizon = height * 0.38;

        // Deep synthwave ambient glow on horizon
        const horizGrad = ctx.createLinearGradient(0, horizon - 120, 0, horizon + 160);
        horizGrad.addColorStop(0, 'transparent');
        horizGrad.addColorStop(0.5, `${primaryColor}35`);
        horizGrad.addColorStop(1, 'transparent');
        ctx.fillStyle = horizGrad;
        ctx.fillRect(0, horizon - 120, width, 280);

        // Cyber glowing wireframe sun on horizon
        const sunRadius = Math.min(width * 0.14, 90);
        const sunGrad = ctx.createRadialGradient(width / 2, horizon, 10, width / 2, horizon, sunRadius);
        sunGrad.addColorStop(0, '#ffffff');
        sunGrad.addColorStop(0.3, primaryColor);
        sunGrad.addColorStop(0.8, '#f43f5e88');
        sunGrad.addColorStop(1, 'transparent');
        ctx.fillStyle = sunGrad;
        ctx.beginPath();
        ctx.arc(width / 2, horizon, sunRadius, Math.PI, 0, false);
        ctx.fill();

        // Sun scanline slats
        ctx.fillStyle = 'rgba(8, 5, 18, 0.75)';
        for (let s = horizon - sunRadius + 15; s < horizon; s += 8) {
          const slatHeight = 2.5 + (horizon - s) * 0.04;
          ctx.fillRect(width / 2 - sunRadius - 10, s, sunRadius * 2 + 20, slatHeight);
        }

        // Perspective horizontal grid lines
        ctx.lineWidth = 1.4;
        for (let y = horizon; y < height; y += 18) {
          const d = (y - horizon) / (height - horizon);
          const expY = horizon + Math.pow(d, 2.2) * (height - horizon);
          const movingY = expY + (gridScroll * d);

          if (movingY <= height) {
            const alpha = Math.min(0.7, d * 0.85);
            ctx.strokeStyle = `rgba(244, 63, 94, ${alpha})`;
            ctx.beginPath();
            ctx.moveTo(0, movingY);
            ctx.lineTo(width, movingY);
            ctx.stroke();
          }
        }

        // Vertical converging lines
        const vanishingX = width / 2;
        const totalVLines = 26;
        for (let i = -totalVLines; i <= totalVLines; i++) {
          const bottomX = vanishingX + (i * width * 0.07);
          const lineAlpha = Math.max(0.12, 0.5 - Math.abs(i) * 0.015);
          ctx.strokeStyle = `rgba(6, 182, 212, ${lineAlpha})`;
          ctx.lineWidth = 1.2;
          ctx.beginPath();
          ctx.moveTo(vanishingX, horizon);
          ctx.lineTo(bottomX, height);
          ctx.stroke();
        }

        // Shooting cyber data packets
        cyberPackets.forEach(p => {
          p.progress += p.speed;
          if (p.progress > 1) {
            p.progress = 0;
            p.xRatio = (Math.floor(Math.random() * 21) - 10) / 10;
          }

          const startD = p.progress;
          const endD = Math.max(0, p.progress - p.length);
          const startY = horizon + Math.pow(startD, 2.2) * (height - horizon);
          const endY = horizon + Math.pow(endD, 2.2) * (height - horizon);

          const bottomStartX = vanishingX + (p.xRatio * width * 0.7);
          const curStartX = vanishingX + (bottomStartX - vanishingX) * startD;
          const curEndX = vanishingX + (bottomStartX - vanishingX) * endD;

          const grad = ctx.createLinearGradient(curEndX, endY, curStartX, startY);
          grad.addColorStop(0, 'transparent');
          grad.addColorStop(1, p.color);

          ctx.strokeStyle = grad;
          ctx.lineWidth = 3.5;
          ctx.beginPath();
          ctx.moveTo(curEndX, endY);
          ctx.lineTo(curStartX, startY);
          ctx.stroke();

          ctx.fillStyle = '#ffffff';
          ctx.beginPath();
          ctx.arc(curStartX, startY, 3, 0, Math.PI * 2);
          ctx.fill();
        });

        // Mouse interactive HUD reticle
        if (mouse.active) {
          ctx.strokeStyle = '#06b6d4';
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.arc(mouse.x, mouse.y, 24, 0, Math.PI * 2);
          ctx.stroke();
          ctx.beginPath();
          ctx.moveTo(mouse.x - 30, mouse.y);
          ctx.lineTo(mouse.x - 10, mouse.y);
          ctx.moveTo(mouse.x + 10, mouse.y);
          ctx.lineTo(mouse.x + 30, mouse.y);
          ctx.moveTo(mouse.x, mouse.y - 30);
          ctx.lineTo(mouse.x, mouse.y - 10);
          ctx.moveTo(mouse.x, mouse.y + 10);
          ctx.lineTo(mouse.x, mouse.y + 30);
          ctx.stroke();
        }
      }

      // MODE: GAMING WARP (Hyperspace Speed Tunnel)
      else if (currentMode === 'gaming_warp') {
        ctx.fillStyle = 'rgba(8, 5, 18, 0.22)';
        ctx.fillRect(0, 0, width, height);

        const centerX = mouse.active ? mouse.x : width / 2;
        const centerY = mouse.active ? mouse.y : height / 2;

        warpStars.forEach(star => {
          star.pz = star.z;
          star.z -= 18;
          if (star.z <= 0) {
            star.z = width;
            star.pz = width;
            star.x = (Math.random() - 0.5) * width * 2;
            star.y = (Math.random() - 0.5) * height * 2;
          }

          const k = 280 / star.z;
          const px = star.x * k + centerX;
          const py = star.y * k + centerY;

          const pk = 280 / star.pz;
          const prevX = star.x * pk + centerX;
          const prevY = star.y * pk + centerY;

          if (px >= 0 && px <= width && py >= 0 && py <= height) {
            const alpha = Math.min(1, (1 - star.z / width) * 1.5);
            ctx.strokeStyle = star.color;
            ctx.lineWidth = Math.min(4, k * 2.5);
            ctx.beginPath();
            ctx.moveTo(prevX, prevY);
            ctx.lineTo(px, py);
            ctx.stroke();
          }
        });

        // Warp center laser flare
        const flare = ctx.createRadialGradient(centerX, centerY, 5, centerX, centerY, 160);
        flare.addColorStop(0, 'rgba(6, 182, 212, 0.45)');
        flare.addColorStop(1, 'transparent');
        ctx.fillStyle = flare;
        ctx.beginPath();
        ctx.arc(centerX, centerY, 160, 0, Math.PI * 2);
        ctx.fill();
      }

      // MODE: GAMING HEX (Cyberpunk Hex Shield Grid)
      else if (currentMode === 'gaming_hex') {
        ctx.clearRect(0, 0, width, height);
        hexPulse += 0.03;
        const hexSize = width < 768 ? 42 : 58;
        const h = hexSize * Math.sqrt(3);

        ctx.lineWidth = 1;
        for (let row = -1; row < height / h + 2; row++) {
          for (let col = -1; col < width / (hexSize * 1.5) + 2; col++) {
            const cx = col * hexSize * 1.5;
            const cy = row * h + (col % 2 === 1 ? h / 2 : 0);

            const distToMouse = mouse.active ? Math.hypot(cx - mouse.x, cy - mouse.y) : 999;
            const isNearMouse = distToMouse < 180;
            const pulseVal = Math.sin(hexPulse + (cx + cy) * 0.005);

            let alpha = 0.08 + pulseVal * 0.04;
            let strokeColor = 'rgba(244, 63, 94, ';
            if (isNearMouse) {
              alpha = (1 - distToMouse / 180) * 0.7;
              strokeColor = 'rgba(6, 182, 212, ';
            }

            ctx.strokeStyle = `${strokeColor}${alpha})`;
            ctx.beginPath();
            for (let a = 0; a < 6; a++) {
              const angle = (Math.PI / 3) * a;
              const hx = cx + hexSize * 0.5 * Math.cos(angle);
              const hy = cy + hexSize * 0.5 * Math.sin(angle);
              if (a === 0) ctx.moveTo(hx, hy);
              else ctx.lineTo(hx, hy);
            }
            ctx.closePath();
            ctx.stroke();

            if (isNearMouse) {
              ctx.fillStyle = `rgba(6, 182, 212, ${alpha * 0.15})`;
              ctx.fill();
            }
          }
        }
      }

      // ===========================================================
      // [CLASSICAL / RETRO UNIX TERMINAL CATEGORY ANIMATIONS]
      // ===========================================================

      // MODE: SCANLINE CRT (Matrix Code Rain with CRT Scanlines)
      else if (currentMode === 'scanline_crt') {
        ctx.fillStyle = 'rgba(5, 14, 8, 0.16)';
        ctx.fillRect(0, 0, width, height);

        ctx.font = `bold ${charSize}px monospace`;

        drops.forEach((d, i) => {
          const x = i * charSize;
          const y = d.y * charSize;
          const char = d.chars[tick % d.chars.length];

          ctx.fillStyle = '#ffffff';
          ctx.shadowColor = '#22c55e';
          ctx.shadowBlur = 8;
          ctx.fillText(char, x, y);

          ctx.shadowBlur = 0;
          ctx.fillStyle = '#22c55e';
          const prevChar = d.chars[(tick + 1) % d.chars.length];
          ctx.fillText(prevChar, x, y - charSize);

          if (y > height && Math.random() > 0.98) {
            d.y = 0;
          }
          d.y += d.speed;
        });

        // CRT horizontal scanline beam
        const scanY = (tick * 3.5) % height;
        const scanGrad = ctx.createLinearGradient(0, scanY - 35, 0, scanY + 35);
        scanGrad.addColorStop(0, 'transparent');
        scanGrad.addColorStop(0.5, 'rgba(34, 197, 94, 0.18)');
        scanGrad.addColorStop(1, 'transparent');
        ctx.fillStyle = scanGrad;
        ctx.fillRect(0, scanY - 35, width, 70);
      }

      // MODE: CLASSIC TERMINAL (Retro Unix Boot Shell & Memory Hex Dump)
      else if (currentMode === 'classic_terminal') {
        ctx.clearRect(0, 0, width, height);

        // Terminal background vignette with subtle green tint
        const termBg = ctx.createRadialGradient(width / 2, height / 2, 50, width / 2, height / 2, width * 0.7);
        termBg.addColorStop(0, 'rgba(5, 20, 10, 0.35)');
        termBg.addColorStop(1, 'rgba(2, 8, 4, 0.85)');
        ctx.fillStyle = termBg;
        ctx.fillRect(0, 0, width, height);

        ctx.font = '13px monospace';
        const startY = 80;
        const lineHeight = 22;

        terminalLogs.forEach((line, idx) => {
          const y = startY + idx * lineHeight;
          if (y < height - 60) {
            ctx.fillStyle = idx === terminalLogs.length - 1 ? '#4ade80' : 'rgba(34, 197, 94, 0.65)';
            ctx.fillText(line, 40, y);
          }
        });

        // Memory Hex Addresses on the right
        ctx.fillStyle = 'rgba(34, 197, 94, 0.35)';
        for (let r = 0; r < 14; r++) {
          const hexAddr = `0x7FFF${(1000 + r * 16).toString(16).toUpperCase()}: ` +
            `${Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase()} ` +
            `${Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase()} ` +
            `${Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase()} ` +
            `${Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase()}`;
          ctx.fillText(hexAddr, width - 260, startY + r * lineHeight);
        }

        // Blinking Prompt cursor
        const blink = Math.floor(tick / 25) % 2 === 0;
        const promptY = startY + terminalLogs.length * lineHeight;
        ctx.fillStyle = '#22c55e';
        ctx.fillText('[root@veylix-node ~]# _' + (blink ? ' █' : ''), 40, promptY);
      }

      // MODE: CLASSIC OSCILLOSCOPE (Vector CRT Audio Sine Wave)
      else if (currentMode === 'classic_oscilloscope') {
        ctx.clearRect(0, 0, width, height);
        oscPhase += 0.04;

        // Oscilloscope graticule grid
        ctx.strokeStyle = 'rgba(34, 197, 94, 0.12)';
        ctx.lineWidth = 1;
        const gridSize = 50;
        for (let x = 0; x < width; x += gridSize) {
          ctx.beginPath();
          ctx.moveTo(x, 0);
          ctx.lineTo(x, height);
          ctx.stroke();
        }
        for (let y = 0; y < height; y += gridSize) {
          ctx.beginPath();
          ctx.moveTo(0, y);
          ctx.lineTo(width, y);
          ctx.stroke();
        }

        // Sine wave vector trace 1
        ctx.strokeStyle = '#22c55e';
        ctx.shadowColor = '#22c55e';
        ctx.shadowBlur = 10;
        ctx.lineWidth = 2.5;
        ctx.beginPath();
        const midY = height / 2;
        const amp1 = mouse.active ? 90 + Math.abs(mouse.y - midY) * 0.2 : 80;

        for (let x = 0; x < width; x += 4) {
          const y = midY + Math.sin(x * 0.008 + oscPhase) * amp1 + Math.sin(x * 0.02 - oscPhase * 1.5) * 25;
          if (x === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();

        // Wave trace 2 (Harmonic secondary wave)
        ctx.strokeStyle = '#86efac';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        for (let x = 0; x < width; x += 5) {
          const y = midY + Math.cos(x * 0.006 - oscPhase * 0.8) * (amp1 * 0.6) + Math.sin(x * 0.015 + oscPhase) * 15;
          if (x === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();
        ctx.shadowBlur = 0;
      }

      // ===========================================================
      // [PROFESSIONAL / ENTERPRISE VPN CATEGORY ANIMATIONS]
      // ===========================================================

      // MODE: PARTICLES (Global VPN Node Constellation)
      else if (currentMode === 'particles') {
        ctx.clearRect(0, 0, width, height);

        // Soft ambient corner cloud
        const bgGlow = ctx.createRadialGradient(width * 0.15, height * 0.25, 20, width * 0.15, height * 0.25, 450);
        bgGlow.addColorStop(0, `${primaryColor}22`);
        bgGlow.addColorStop(1, 'transparent');
        ctx.fillStyle = bgGlow;
        ctx.fillRect(0, 0, width, height);

        // Draw node links
        for (let i = 0; i < nodes.length; i++) {
          const a = nodes[i];
          a.x += a.vx;
          a.y += a.vy;

          if (a.x < 0 || a.x > width) a.vx *= -1;
          if (a.y < 0 || a.y > height) a.vy *= -1;
          a.pulse += 0.04;

          for (let j = i + 1; j < nodes.length; j++) {
            const b = nodes[j];
            const dist = Math.hypot(a.x - b.x, a.y - b.y);
            const maxDist = width < 768 ? 110 : 155;

            if (dist < maxDist) {
              const alpha = (1 - dist / maxDist) * 0.42;
              ctx.strokeStyle = `rgba(16, 185, 129, ${alpha})`;
              ctx.lineWidth = 1.2;
              ctx.beginPath();
              ctx.moveTo(a.x, a.y);
              ctx.lineTo(b.x, b.y);
              ctx.stroke();
            }
          }

          // Mouse link
          if (mouse.active) {
            const mDist = Math.hypot(a.x - mouse.x, a.y - mouse.y);
            const mMax = 180;
            if (mDist < mMax) {
              const mAlpha = (1 - mDist / mMax) * 0.65;
              ctx.strokeStyle = `rgba(56, 189, 248, ${mAlpha})`;
              ctx.lineWidth = 2;
              ctx.beginPath();
              ctx.moveTo(a.x, a.y);
              ctx.lineTo(mouse.x, mouse.y);
              ctx.stroke();
              a.x += (mouse.x - a.x) * 0.008;
              a.y += (mouse.y - a.y) * 0.008;
            }
          }

          // Node body
          const haloSize = a.radius * 2.8 + Math.sin(a.pulse) * 1.5;
          ctx.fillStyle = `${a.color}25`;
          ctx.beginPath();
          ctx.arc(a.x, a.y, haloSize, 0, Math.PI * 2);
          ctx.fill();

          ctx.fillStyle = a.color;
          ctx.beginPath();
          ctx.arc(a.x, a.y, a.radius, 0, Math.PI * 2);
          ctx.fill();
        }

        // Packets
        networkPackets.forEach(p => {
          p.progress += p.speed;
          if (p.progress >= 1) {
            p.progress = 0;
            p.fromIdx = Math.floor(Math.random() * nodes.length);
            p.toIdx = Math.floor(Math.random() * nodes.length);
          }

          const nodeA = nodes[p.fromIdx];
          const nodeB = nodes[p.toIdx];
          if (nodeA && nodeB) {
            const dist = Math.hypot(nodeA.x - nodeB.x, nodeA.y - nodeB.y);
            if (dist < 220) {
              const px = nodeA.x + (nodeB.x - nodeA.x) * p.progress;
              const py = nodeA.y + (nodeB.y - nodeA.y) * p.progress;

              ctx.fillStyle = p.color;
              ctx.beginPath();
              ctx.arc(px, py, 2.8, 0, Math.PI * 2);
              ctx.fill();
            }
          }
        });
      }

      // MODE: PRO WORLD MAP (Global Server Route Map & Live Latency Pins)
      else if (currentMode === 'pro_worldmap') {
        ctx.clearRect(0, 0, width, height);

        // Blueprint grid
        ctx.strokeStyle = 'rgba(16, 185, 129, 0.08)';
        ctx.lineWidth = 1;
        for (let x = 0; x < width; x += 60) {
          ctx.beginPath();
          ctx.moveTo(x, 0);
          ctx.lineTo(x, height);
          ctx.stroke();
        }
        for (let y = 0; y < height; y += 60) {
          ctx.beginPath();
          ctx.moveTo(0, y);
          ctx.lineTo(width, y);
          ctx.stroke();
        }

        // Draw flight arcs between world hubs
        for (let i = 0; i < worldHubs.length; i++) {
          const h1 = worldHubs[i];
          const p1x = h1.x * width;
          const p1y = h1.y * height;

          for (let j = i + 1; j < worldHubs.length; j++) {
            const h2 = worldHubs[j];
            const p2x = h2.x * width;
            const p2y = h2.y * height;

            const midX = (p1x + p2x) / 2;
            const midY = Math.min(p1y, p2y) - 60;

            ctx.strokeStyle = 'rgba(16, 185, 129, 0.25)';
            ctx.setLineDash([4, 4]);
            ctx.beginPath();
            ctx.moveTo(p1x, p1y);
            ctx.quadraticCurveTo(midX, midY, p2x, p2y);
            ctx.stroke();
            ctx.setLineDash([]);
          }

          // Hub pulsing beacon
          const pulse = (tick * 0.05) % (Math.PI * 2);
          ctx.strokeStyle = h1.color;
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.arc(p1x, p1y, 8 + Math.sin(pulse) * 4, 0, Math.PI * 2);
          ctx.stroke();

          ctx.fillStyle = h1.color;
          ctx.beginPath();
          ctx.arc(p1x, p1y, 4, 0, Math.PI * 2);
          ctx.fill();

          // Hub Tag
          ctx.font = 'bold 11px monospace';
          ctx.fillStyle = '#ffffff';
          ctx.fillText(`${h1.name} [${h1.ping}]`, p1x + 12, p1y + 4);
        }
      }

      // MODE: PRO FIBER (High-Speed Fiber Optic Data Streams)
      else if (currentMode === 'pro_fiber') {
        ctx.clearRect(0, 0, width, height);

        fiberBuses.forEach(bus => {
          // Bus line
          ctx.strokeStyle = 'rgba(16, 185, 129, 0.15)';
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.moveTo(0, bus.y);
          ctx.lineTo(width, bus.y);
          ctx.stroke();

          // Data pulses
          bus.pulses.forEach((px, pidx) => {
            bus.pulses[pidx] = (px + bus.speed) % width;
            const curX = bus.pulses[pidx];

            const pGrad = ctx.createLinearGradient(curX - 60, bus.y, curX, bus.y);
            pGrad.addColorStop(0, 'transparent');
            pGrad.addColorStop(1, bus.color);

            ctx.strokeStyle = pGrad;
            ctx.lineWidth = 3;
            ctx.beginPath();
            ctx.moveTo(curX - 60, bus.y);
            ctx.lineTo(curX, bus.y);
            ctx.stroke();

            ctx.fillStyle = '#ffffff';
            ctx.beginPath();
            ctx.arc(curX, bus.y, 2.5, 0, Math.PI * 2);
            ctx.fill();
          });
        });
      }

      // ===========================================================
      // [DARK / STEALTH DEEP SPACE CATEGORY ANIMATIONS]
      // ===========================================================

      // MODE: SUBTLE ORBS (Deep Cosmic Breathing Nebula & Stars)
      else if (currentMode === 'subtle_orbs') {
        ctx.clearRect(0, 0, width, height);
        const t = tick * 0.004;

        // Nebula 1: Deep Emerald / Teal
        const neb1X = width * 0.35 + Math.sin(t) * 120;
        const neb1Y = height * 0.3 + Math.cos(t * 0.9) * 80;
        const nebGrad1 = ctx.createRadialGradient(neb1X, neb1Y, 20, neb1X, neb1Y, 550);
        nebGrad1.addColorStop(0, `${primaryColor}28`);
        nebGrad1.addColorStop(0.5, `${accentColor}15`);
        nebGrad1.addColorStop(1, 'transparent');
        ctx.fillStyle = nebGrad1;
        ctx.fillRect(0, 0, width, height);

        // Nebula 2: Deep Indigo / Violet
        const neb2X = width * 0.78 + Math.cos(t * 0.8) * 110;
        const neb2Y = height * 0.72 + Math.sin(t * 0.7) * 90;
        const nebGrad2 = ctx.createRadialGradient(neb2X, neb2Y, 20, neb2X, neb2Y, 600);
        nebGrad2.addColorStop(0, 'rgba(139, 92, 246, 0.22)');
        nebGrad2.addColorStop(0.6, 'rgba(56, 189, 248, 0.12)');
        nebGrad2.addColorStop(1, 'transparent');
        ctx.fillStyle = nebGrad2;
        ctx.fillRect(0, 0, width, height);

        // Stars
        stars.forEach(s => {
          s.alpha += s.pulseSpeed;
          if (s.alpha > 0.95 || s.alpha < 0.2) s.pulseSpeed *= -1;

          const curAlpha = Math.max(0.1, Math.min(1, s.alpha));
          ctx.fillStyle = `rgba(255, 255, 255, ${curAlpha})`;
          ctx.beginPath();
          ctx.arc(s.x, s.y, s.size, 0, Math.PI * 2);
          ctx.fill();

          if (s.hasCrossGlow && s.size > 1.8) {
            const spikeLen = s.size * 3.5;
            ctx.strokeStyle = `rgba(255, 255, 255, ${curAlpha * 0.6})`;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(s.x - spikeLen, s.y);
            ctx.lineTo(s.x + spikeLen, s.y);
            ctx.moveTo(s.x, s.y - spikeLen);
            ctx.lineTo(s.x, s.y + spikeLen);
            ctx.stroke();
          }
        });

        // Meteors
        nextMeteorTimer--;
        if (nextMeteorTimer <= 0 && !meteor) {
          meteor = {
            x: Math.random() * width * 0.8,
            y: Math.random() * height * 0.3,
            length: 120 + Math.random() * 80,
            speed: 12 + Math.random() * 8,
            alpha: 1
          };
          nextMeteorTimer = 180 + Math.floor(Math.random() * 240);
        }

        if (meteor) {
          meteor.x += meteor.speed;
          meteor.y += meteor.speed * 0.65;
          meteor.alpha -= 0.02;

          const mGrad = ctx.createLinearGradient(
            meteor.x - meteor.length,
            meteor.y - meteor.length * 0.65,
            meteor.x,
            meteor.y
          );
          mGrad.addColorStop(0, 'transparent');
          mGrad.addColorStop(1, `rgba(255, 255, 255, ${Math.max(0, meteor.alpha)})`);

          ctx.strokeStyle = mGrad;
          ctx.lineWidth = 2.5;
          ctx.beginPath();
          ctx.moveTo(meteor.x - meteor.length, meteor.y - meteor.length * 0.65);
          ctx.lineTo(meteor.x, meteor.y);
          ctx.stroke();

          if (meteor.alpha <= 0 || meteor.x > width || meteor.y > height) {
            meteor = null;
          }
        }
      }

      // MODE: DARK STEALTH RADAR (Covert 360-Degree Radar Sweep)
      else if (currentMode === 'dark_stealth_radar') {
        ctx.clearRect(0, 0, width, height);
        radarAngle = (radarAngle + 0.02) % (Math.PI * 2);
        const rcx = width / 2;
        const rcy = height / 2;
        const maxR = Math.min(width, height) * 0.44;

        // Concentric range circles
        ctx.strokeStyle = 'rgba(56, 189, 248, 0.15)';
        ctx.lineWidth = 1;
        [0.25, 0.5, 0.75, 1].forEach(frac => {
          ctx.beginPath();
          ctx.arc(rcx, rcy, maxR * frac, 0, Math.PI * 2);
          ctx.stroke();
        });

        // Crosshairs
        ctx.beginPath();
        ctx.moveTo(rcx - maxR, rcy);
        ctx.lineTo(rcx + maxR, rcy);
        ctx.moveTo(rcx, rcy - maxR);
        ctx.lineTo(rcx, rcy + maxR);
        ctx.stroke();

        // Radar sweep pie sector
        ctx.save();
        ctx.beginPath();
        ctx.moveTo(rcx, rcy);
        ctx.arc(rcx, rcy, maxR, radarAngle - 0.45, radarAngle);
        ctx.closePath();
        const sweepGrad = ctx.createRadialGradient(rcx, rcy, 10, rcx, rcy, maxR);
        sweepGrad.addColorStop(0, 'rgba(56, 189, 248, 0.25)');
        sweepGrad.addColorStop(1, 'rgba(56, 189, 248, 0.05)');
        ctx.fillStyle = sweepGrad;
        ctx.fill();
        ctx.restore();

        // Stealth blips
        stealthBlips.forEach(b => {
          const angleDiff = Math.abs((radarAngle - b.angle + Math.PI * 2) % (Math.PI * 2));
          if (angleDiff < 0.2) {
            b.alpha = 1;
          } else {
            b.alpha = Math.max(0, b.alpha - 0.008);
          }

          if (b.alpha > 0.05) {
            const bx = rcx + maxR * b.dist * Math.cos(b.angle);
            const by = rcy + maxR * b.dist * Math.sin(b.angle);

            ctx.fillStyle = `rgba(56, 189, 248, ${b.alpha})`;
            ctx.beginPath();
            ctx.arc(bx, by, 4, 0, Math.PI * 2);
            ctx.fill();

            ctx.font = '10px monospace';
            ctx.fillText(b.label, bx + 8, by + 3);
          }
        });
      }

      // MODE: DARK STARFALL (Deep Space Starfall Shower)
      else if (currentMode === 'dark_starfall') {
        ctx.clearRect(0, 0, width, height);

        stars.forEach(s => {
          s.y = (s.y + s.size * 0.8) % height;
          ctx.fillStyle = `rgba(255, 255, 255, ${s.alpha * 0.8})`;
          ctx.beginPath();
          ctx.arc(s.x, s.y, s.size, 0, Math.PI * 2);
          ctx.fill();
        });
      }

      // ===========================================================
      // [LIGHT / DAYLIGHT CATEGORY ANIMATIONS]
      // ===========================================================

      // MODE: AURORA WAVE (Fluid Luminous Daylight Ribbons & Bokeh)
      else if (currentMode === 'aurora_wave') {
        ctx.clearRect(0, 0, width, height);
        auroraPhase += 0.016;

        bokehOrbs.forEach(b => {
          b.x += b.vx;
          b.y += b.vy;
          if (b.x < -100) b.x = width + 100;
          if (b.x > width + 100) b.x = -100;
          if (b.y < -100) b.y = height + 100;
          if (b.y > height + 100) b.y = -100;

          const bokehGrad = ctx.createRadialGradient(b.x, b.y, 10, b.x, b.y, b.radius);
          bokehGrad.addColorStop(0, `${b.color}25`);
          bokehGrad.addColorStop(1, 'transparent');
          ctx.fillStyle = bokehGrad;
          ctx.beginPath();
          ctx.arc(b.x, b.y, b.radius, 0, Math.PI * 2);
          ctx.fill();
        });

        // Wave Ribbon 1
        ctx.beginPath();
        ctx.moveTo(0, height * 0.28);
        for (let x = 0; x <= width; x += 30) {
          const y = height * 0.28 +
            Math.sin(x * 0.003 + auroraPhase) * 70 +
            Math.cos(x * 0.002 - auroraPhase * 0.6) * 45;
          ctx.lineTo(x, y);
        }
        ctx.lineTo(width, height);
        ctx.lineTo(0, height);
        ctx.closePath();
        const wave1Grad = ctx.createLinearGradient(0, height * 0.1, width, height);
        wave1Grad.addColorStop(0, `${primaryColor}22`);
        wave1Grad.addColorStop(0.5, `${accentColor}14`);
        wave1Grad.addColorStop(1, 'transparent');
        ctx.fillStyle = wave1Grad;
        ctx.fill();

        // Wave Ribbon 2
        ctx.beginPath();
        ctx.moveTo(0, height * 0.52);
        for (let x = 0; x <= width; x += 35) {
          const y = height * 0.52 +
            Math.cos(x * 0.0035 - auroraPhase * 0.8) * 80 +
            Math.sin(x * 0.002 + auroraPhase) * 40;
          ctx.lineTo(x, y);
        }
        ctx.lineTo(width, height);
        ctx.lineTo(0, height);
        ctx.closePath();
        const wave2Grad = ctx.createLinearGradient(0, height * 0.4, width, height);
        wave2Grad.addColorStop(0, 'rgba(56, 189, 248, 0.20)');
        wave2Grad.addColorStop(1, 'transparent');
        ctx.fillStyle = wave2Grad;
        ctx.fill();
      }

      // MODE: LIGHT CAUSTICS (Prismatic Sunlight Refraction)
      else if (currentMode === 'light_caustics') {
        ctx.clearRect(0, 0, width, height);
        causticsTime += 0.02;

        // Prismatic light refraction rays from top
        for (let r = 0; r < 6; r++) {
          const rayX = (width * 0.2) + r * (width * 0.15) + Math.sin(causticsTime + r) * 40;
          const rayGrad = ctx.createLinearGradient(rayX, 0, rayX + 60, height);
          rayGrad.addColorStop(0, 'rgba(56, 189, 248, 0.15)');
          rayGrad.addColorStop(0.5, 'rgba(16, 185, 129, 0.08)');
          rayGrad.addColorStop(1, 'transparent');

          ctx.fillStyle = rayGrad;
          ctx.beginPath();
          ctx.moveTo(rayX - 30, 0);
          ctx.lineTo(rayX + 90, 0);
          ctx.lineTo(rayX + 220, height);
          ctx.lineTo(rayX - 80, height);
          ctx.closePath();
          ctx.fill();
        }
      }

      // MODE: LIGHT GEOMETRIC (Clean Minimalist Floating Origami)
      else if (currentMode === 'light_geometric') {
        ctx.clearRect(0, 0, width, height);

        geometricShapes.forEach(s => {
          s.x = (s.x + s.vx + width) % width;
          s.y = (s.y + s.vy + height) % height;
          s.rot += s.vrot;

          ctx.save();
          ctx.translate(s.x, s.y);
          ctx.rotate(s.rot);

          ctx.strokeStyle = 'rgba(2, 132, 199, 0.28)';
          ctx.fillStyle = 'rgba(2, 132, 199, 0.04)';
          ctx.lineWidth = 1.2;

          if (s.type === 'cube') {
            ctx.strokeRect(-s.size / 2, -s.size / 2, s.size, s.size);
            ctx.fillRect(-s.size / 2, -s.size / 2, s.size, s.size);
            // Inner diamond
            ctx.beginPath();
            ctx.moveTo(0, -s.size / 2);
            ctx.lineTo(s.size / 2, 0);
            ctx.lineTo(0, s.size / 2);
            ctx.lineTo(-s.size / 2, 0);
            ctx.closePath();
            ctx.stroke();
          } else if (s.type === 'tetra') {
            ctx.beginPath();
            ctx.moveTo(0, -s.size / 2);
            ctx.lineTo(s.size / 2, s.size / 2);
            ctx.lineTo(-s.size / 2, s.size / 2);
            ctx.closePath();
            ctx.stroke();
            ctx.fill();
          } else {
            // Hexagon
            ctx.beginPath();
            for (let a = 0; a < 6; a++) {
              const ang = (Math.PI / 3) * a;
              const px = (s.size / 2) * Math.cos(ang);
              const py = (s.size / 2) * Math.sin(ang);
              if (a === 0) ctx.moveTo(px, py);
              else ctx.lineTo(px, py);
            }
            ctx.closePath();
            ctx.stroke();
            ctx.fill();
          }
          ctx.restore();
        });
      }

      animationFrameId = requestAnimationFrame(render);
    };

    render();

    return () => {
      window.removeEventListener('resize', handleResize);
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseleave', handleMouseLeave);
      cancelAnimationFrame(animationFrameId);
    };
  }, [
    themeConfig.preset,
    themeConfig.primaryColor,
    themeConfig.accentColor,
    themeConfig.enableAnimations,
    themeConfig.bgAnimationType
  ]);

  return (
    <canvas
      ref={canvasRef}
      className="fixed inset-0 pointer-events-none z-0 transition-opacity duration-500"
      style={{
        opacity: themeConfig.enableAnimations ? 1 : 0
      }}
    />
  );
};
