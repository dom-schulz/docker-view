{
  description = "dkrv: a terminal UI for Docker that groups containers by compose project";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = "0.1.0";
    in
    {
      packages = forAll (pkgs: rec {
        docker-view = pkgs.buildGoModule {
          pname = "docker-view";
          inherit version;
          src = self;
          vendorHash = "sha256-An2nvY16w49nki0ulkA2msjTwgUXW5XWE3smATW1XBc=";
          subPackages = [ "cmd/dkrv" ];
          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          postInstall = ''
            install -Dm644 assets/dkrv.svg $out/share/icons/hicolor/scalable/apps/dkrv.svg
          '';
          meta = {
            description = "Terminal UI for Docker that groups containers by compose project";
            homepage = "https://github.com/dom-schulz/docker-view";
            license = pkgs.lib.licenses.mit;
            mainProgram = "dkrv";
          };
        };
        default = docker-view;
      });

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [ go gopls ];
          env.CGO_ENABLED = 0;
        };
      });

      formatter = forAll (pkgs: pkgs.nixpkgs-fmt);
    };
}
