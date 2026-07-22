export function createBrowserCapacity(maxOpenBrowsers) {
  let active = 0;

  return {
    tryAcquire() {
      if (active >= maxOpenBrowsers) {
        return null;
      }
      active += 1;
      let released = false;
      return () => {
        if (!released) {
          released = true;
          active -= 1;
        }
      };
    },
    active() {
      return active;
    },
    limit() {
      return maxOpenBrowsers;
    },
  };
}
