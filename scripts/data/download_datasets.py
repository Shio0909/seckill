#!/usr/bin/env python3
"""
数据集下载脚本
支持下载 MovieLens 和准备 Kaggle Event Recommendation 数据

用法:
    python download_datasets.py --dataset movielens
    python download_datasets.py --dataset all
"""

import os
import sys
import zipfile
import argparse
import urllib.request
from pathlib import Path


DATA_DIR = Path(__file__).parent.parent.parent / "data"

DATASETS = {
    "movielens-100k": {
        "url": "https://files.grouplens.org/datasets/movielens/ml-100k.zip",
        "desc": "MovieLens 100K - 快速验证",
        "size": "5MB",
    },
    "movielens-1m": {
        "url": "https://files.grouplens.org/datasets/movielens/ml-1m.zip",
        "desc": "MovieLens 1M - 推荐使用",
        "size": "6MB",
    },
    "movielens-10m": {
        "url": "https://files.grouplens.org/datasets/movielens/ml-10m.zip",
        "desc": "MovieLens 10M - 大规模实验",
        "size": "63MB",
    },
    "movielens-25m": {
        "url": "https://files.grouplens.org/datasets/movielens/ml-25m.zip",
        "desc": "MovieLens 25M - 论文级别",
        "size": "250MB",
    },
}


def download_file(url: str, save_path: Path, desc: str = "") -> bool:
    """下载文件并显示进度"""
    print(f"\n📥 下载: {desc or url}")
    print(f"   保存到: {save_path}")
    
    save_path.parent.mkdir(parents=True, exist_ok=True)
    
    try:
        def progress_hook(count, block_size, total_size):
            percent = int(count * block_size * 100 / total_size) if total_size > 0 else 0
            percent = min(percent, 100)
            bar = "=" * (percent // 2) + " " * (50 - percent // 2)
            sys.stdout.write(f"\r   [{bar}] {percent}%")
            sys.stdout.flush()
        
        urllib.request.urlretrieve(url, save_path, progress_hook)
        print("\n   ✅ 下载完成!")
        return True
    except Exception as e:
        print(f"\n   ❌ 下载失败: {e}")
        return False


def extract_zip(zip_path: Path, extract_to: Path) -> bool:
    """解压ZIP文件"""
    print(f"📦 解压: {zip_path}")
    try:
        with zipfile.ZipFile(zip_path, 'r') as zip_ref:
            zip_ref.extractall(extract_to)
        print(f"   ✅ 解压完成!")
        return True
    except Exception as e:
        print(f"   ❌ 解压失败: {e}")
        return False


def download_movielens(version: str = "1m"):
    """下载 MovieLens 数据集"""
    key = f"movielens-{version}"
    if key not in DATASETS:
        print(f"❌ 未知版本: {version}")
        print(f"   可用版本: 100k, 1m, 10m, 25m")
        return False
    
    dataset = DATASETS[key]
    zip_path = DATA_DIR / f"ml-{version}.zip"
    
    # 检查是否已下载
    extract_dir = DATA_DIR / f"ml-{version}"
    if extract_dir.exists():
        print(f"✅ 已存在: {extract_dir}")
        return True
    
    # 下载
    if not download_file(dataset["url"], zip_path, dataset["desc"]):
        return False
    
    # 解压
    if not extract_zip(zip_path, DATA_DIR):
        return False
    
    # 删除ZIP
    zip_path.unlink()
    print(f"🗑️  已删除ZIP文件")
    
    return True


def print_kaggle_instructions():
    """打印Kaggle数据集下载说明"""
    print("""
╔══════════════════════════════════════════════════════════════════════════════╗
║                    📊 Kaggle Event Recommendation 数据集                       ║
╠══════════════════════════════════════════════════════════════════════════════╣
║                                                                              ║
║  这个数据集需要Kaggle账号手动下载，因为需要同意比赛规则。                         ║
║                                                                              ║
║  下载步骤:                                                                    ║
║  1. 访问: https://www.kaggle.com/c/event-recommendation-engine-challenge     ║
║  2. 点击 "Join Competition" 并同意规则                                        ║
║  3. 点击 "Data" 标签页                                                        ║
║  4. 下载以下文件:                                                             ║
║     - users.csv.gz (~30MB)                                                   ║
║     - events.csv.gz (~1.4GB)                                                 ║
║     - train.csv (~500MB)                                                     ║
║     - event_attendees.csv.gz (~200MB)                                        ║
║                                                                              ║
║  5. 将下载的文件放到: data/kaggle-events/ 目录                                 ║
║                                                                              ║
║  或者使用Kaggle CLI:                                                          ║
║  $ pip install kaggle                                                        ║
║  $ kaggle competitions download -c event-recommendation-engine-challenge     ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝
    """)


def check_kaggle_data() -> bool:
    """检查Kaggle数据是否存在"""
    kaggle_dir = DATA_DIR / "kaggle-events"
    required_files = ["users.csv", "events.csv", "train.csv"]
    
    if not kaggle_dir.exists():
        return False
    
    for f in required_files:
        # 检查 .csv 或 .csv.gz
        if not (kaggle_dir / f).exists() and not (kaggle_dir / f"{f}.gz").exists():
            return False
    
    return True


def print_data_status():
    """打印数据状态"""
    print("\n📊 数据集状态:")
    print("-" * 60)
    
    # MovieLens
    for version in ["100k", "1m", "10m", "25m"]:
        path = DATA_DIR / f"ml-{version}"
        status = "✅ 已下载" if path.exists() else "❌ 未下载"
        key = f"movielens-{version}"
        desc = DATASETS[key]["desc"]
        size = DATASETS[key]["size"]
        print(f"  {status} MovieLens {version.upper()} ({size}) - {desc}")
    
    # Kaggle
    status = "✅ 已下载" if check_kaggle_data() else "❌ 未下载"
    print(f"  {status} Kaggle Event Recommendation (~2GB) - 真实活动推荐数据")
    
    print("-" * 60)


def main():
    parser = argparse.ArgumentParser(description="下载推荐系统数据集")
    parser.add_argument(
        "--dataset", "-d",
        choices=["movielens", "kaggle", "all", "status"],
        default="status",
        help="要下载的数据集"
    )
    parser.add_argument(
        "--version", "-v",
        default="1m",
        help="MovieLens版本 (100k, 1m, 10m, 25m)"
    )
    
    args = parser.parse_args()
    
    print("=" * 60)
    print("🎬 推荐系统数据集下载工具")
    print("=" * 60)
    
    if args.dataset == "status":
        print_data_status()
        print("\n使用示例:")
        print("  python download_datasets.py -d movielens -v 1m")
        print("  python download_datasets.py -d kaggle")
        return
    
    if args.dataset in ["movielens", "all"]:
        download_movielens(args.version)
    
    if args.dataset in ["kaggle", "all"]:
        if check_kaggle_data():
            print("✅ Kaggle数据已存在")
        else:
            print_kaggle_instructions()
    
    print_data_status()


if __name__ == "__main__":
    main()
