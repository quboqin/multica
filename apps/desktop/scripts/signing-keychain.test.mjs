// @vitest-environment node

import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname } from "node:path";
import { runInNewContext } from "node:vm";
import { expect, it } from "vitest";

const require = createRequire(import.meta.url);
const builderRequire = createRequire(require.resolve("electron-builder"));
const signingPath = builderRequire.resolve("app-builder-lib/out/codeSign/macCodeSign.js");
const signingRequire = createRequire(signingPath);

it.each([false, true])("uses the temporary keychain password for access control (installer: %s)", async (withInstaller) => {
  const commands = [];
  let keychainPassword;
  const exports = {};
  // Exercise the installed, patched signing implementation, isolating every
  // security command and certificate import from the host and real secrets.
  runInNewContext(readFileSync(signingPath, "utf8"), {
    exports,
    __dirname: dirname(signingPath),
    process: { platform: "darwin", env: { TRAVIS: "true" } },
    require(id) {
      if (id === "builder-util") {
        return {
          async exec(file, args) {
            expect(file).toBe("/usr/bin/security");
            commands.push(args);
            if (args[0] === "create-keychain") {
              keychainPassword = args[args.indexOf("-p") + 1];
            }
            if (args[0] === "set-key-partition-list") {
              expect(args[args.indexOf("-k") + 1]).toBe(keychainPassword);
            }
            return "";
          },
        };
      }
      if (id === "./codesign") {
        return { importCertificate: async (link) => link };
      }
      return signingRequire(id);
    },
  });

  await exports.createKeychain({
    tmpDir: {},
    currentDir: "/test/multica",
    cscLink: "/test/application.p12",
    cscKeyPassword: "application-fixture-password",
    ...(withInstaller ? {
      cscILink: "/test/installer.p12",
      cscIKeyPassword: "installer-fixture-password",
    } : {}),
  });

  const imports = commands.filter(([command]) => command === "import");
  const expectedPasswords = withInstaller
    ? ["application-fixture-password", "installer-fixture-password"]
    : ["application-fixture-password"];
  expect(imports.map((args) => args[args.indexOf("-P") + 1])).toEqual(expectedPasswords);
  expect(keychainPassword).toBeTruthy();
  expect(expectedPasswords).not.toContain(keychainPassword);
  expect(commands.filter(([command]) => command === "set-key-partition-list")).toHaveLength(imports.length);
});
