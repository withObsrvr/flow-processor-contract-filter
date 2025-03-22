{
  description = "Obsrvr Flow Plugin: Contract Filter Processor";

  nixConfig = {
    allow-dirty = true;  # Helpful during local development
  };

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages = {
          default = pkgs.buildGoModule {
            pname = "flow-processor-contract-filter";
            version = "1.0.0";
            src = ./.;
            
            # Skip vendoring check initially (or if using vendored deps)
            vendorHash = null;
            
            # Disable hardening required for Go plugins
            hardeningDisable = [ "all" ];
            
            # Set up the build environment for plugin compilation
            preBuild = ''
              export CGO_ENABLED=1
            '';
            
            # Build the plugin as a shared library
            buildPhase = ''
              runHook preBuild
              # Use -mod=vendor if you have vendored dependencies
              go build -buildmode=plugin -o flow-processor-contract-filter.so .
              runHook postBuild
            '';

            # Custom install phase to place the plugin and go.mod
            installPhase = ''
              runHook preInstall
              mkdir -p $out/lib
              cp flow-processor-contract-filter.so $out/lib/
              mkdir -p $out/share
              cp go.mod $out/share/
              if [ -f go.sum ]; then
                cp go.sum $out/share/
              fi
              runHook postInstall
            '';
            
            nativeBuildInputs = [ pkgs.pkg-config ];
            buildInputs = [
              # Include any C library dependencies as needed
            ];
          };
        };

        # Development shell with necessary tools
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_21  # Replace with an overlay if you need a different Go version
            pkg-config
            git
            gopls
            delve
          ];
          shellHook = ''
            export CGO_ENABLED=1
            if [ ! -d vendor ]; then
              echo "Vendoring dependencies..."
              go mod tidy
              go mod vendor
            fi
            echo "Development environment ready!"
            echo "To build the plugin manually: go build -buildmode=plugin -o flow-processor-contract-filter.so ."
          '';
        };
      }
    );
} 