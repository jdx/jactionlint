node -e "if (process.env.MBX_GC_AUTO !== '0') { console.error('object mode must disable automatic GC on GitHub-hosted runners'); process.exit(1) }"
