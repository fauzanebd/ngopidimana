import { useEffect, useState } from "react";
import { checkHealth } from "../api/client";

// Drives the header's connectivity dot. It asks the API rather than inferring from the
// last request, because a request that hangs produces no error to infer from — which is
// exactly how the header once claimed "API connected" over an empty queue.
export function useApiHealth(intervalMs = 20_000) {
  const [healthy, setHealthy] = useState(true);
  useEffect(() => {
    let active = true;
    const check = async () => {
      const reachable = await checkHealth();
      if (active) setHealthy(reachable);
    };
    void check();
    const timer = window.setInterval(check, intervalMs);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [intervalMs]);
  return healthy;
}
