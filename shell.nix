with import <nixpkgs> { };
{
  devEnv = stdenv.mkDerivation {
    name = "dev";
    buildInputs = [ stdenv go_1_21 glibc ];
    shellHook = ''
      return
    '';
  };
}
