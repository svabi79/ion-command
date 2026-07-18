import unreal


ROOT = "/Game/ION"


def ensure_directory(path: str) -> None:
    if not unreal.EditorAssetLibrary.does_directory_exist(path):
        unreal.EditorAssetLibrary.make_directory(path)


def load_class(module: str, class_name: str):
    result = unreal.load_class(None, f"/Script/{module}.{class_name}")
    if result is None:
        raise RuntimeError(f"C++ class is not available: {module}.{class_name}")
    return result


def save_asset(path: str) -> None:
    if not unreal.EditorAssetLibrary.save_asset(path, only_if_is_dirty=False):
        raise RuntimeError(f"Could not save asset: {path}")

