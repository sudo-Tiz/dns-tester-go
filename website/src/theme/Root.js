import React, { useEffect } from 'react';

const k = ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 'b', 'a'];

export default function Root({ children }) {
  useEffect(() => {
    let i = 0;
    const h = (e) => {
      if (e.key.toLowerCase() === k[i].toLowerCase()) {
        if (++i === k.length) {
          const m = document.createElement('div');
          m.style.cssText = 'position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);background:#000;color:#0f0;padding:20px;border:2px solid #0f0;border-radius:5px;font-family:monospace;z-index:9999;text-align:center';
          m.innerHTML = '🎮 KONAMI<br/><small>F5 to exit</small>';
          document.body.appendChild(m);
          document.body.style.cssText = 'background:#000!important;color:#0f0!important;font-family:monospace!important';
          setTimeout(() => m.remove(), 3000);
          i = 0;
        }
      } else i = 0;
    };
    document.addEventListener('keydown', h);
    return () => document.removeEventListener('keydown', h);
  }, []);

  return <>{children}</>;
}
