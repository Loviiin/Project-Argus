import os
import json
import argparse
from pathlib import Path

import torch
import torch.nn as nn
from torch.utils.data import Dataset, DataLoader
from torchvision import models, transforms
from PIL import Image

class RotationCaptchaDataset(Dataset):
    def __init__(self, dataset_dir):
        self.dataset_dir = Path(dataset_dir)
        self.samples = []
        
        # Find all label JSONs
        for label_path in self.dataset_dir.glob("*_label.json"):
            base_id = label_path.name.replace("_label.json", "")
            
            with open(label_path, 'r') as f:
                data = json.load(f)
            
            # Use 'angle' key, assume 0 if not found
            angle = data.get("angle", 0)
            
            outer_path = self.dataset_dir / f"{base_id}_outer.jpg"
            inner_path = self.dataset_dir / f"{base_id}_inner.jpg"
            
            if outer_path.exists() and inner_path.exists():
                self.samples.append({
                    "outer": str(outer_path),
                    "inner": str(inner_path),
                    "angle": angle / 360.0  # Normalize angle to [0, 1]
                })
        
        # Define image transforms
        self.transform = transforms.Compose([
            transforms.Resize((224, 224)),
            transforms.ToTensor(), # Scales [0, 255] to [0.0, 1.0]
        ])

    def __len__(self):
        return len(self.samples)

    def __getitem__(self, idx):
        sample = self.samples[idx]
        
        outer_img = Image.open(sample["outer"]).convert('RGB')
        inner_img = Image.open(sample["inner"]).convert('RGB')
        
        outer_tensor = self.transform(outer_img)
        inner_tensor = self.transform(inner_img)
        
        # Stack on channel dimension: [6, 224, 224]
        stacked_tensor = torch.cat([outer_tensor, inner_tensor], dim=0)
        
        target = torch.tensor([sample["angle"]], dtype=torch.float32)
        
        return stacked_tensor, target

def get_model():
    model = models.resnet18(weights=None)
    
    # Modify conv1: in_channels=6 instead of 3
    original_conv1 = model.conv1
    model.conv1 = nn.Conv2d(
        in_channels=6, 
        out_channels=original_conv1.out_channels, 
        kernel_size=original_conv1.kernel_size, 
        stride=original_conv1.stride, 
        padding=original_conv1.padding, 
        bias=original_conv1.bias is not None
    )
    
    # Modify fc for regression (1 output)
    model.fc = nn.Linear(model.fc.in_features, 1)
    
    return model

def main():
    parser = argparse.ArgumentParser(description="Train Rotation Captcha Model")
    
    current_dir = Path(__file__).parent.resolve()
    
    parser.add_argument(
        "--dataset_dir",
        type=str,
        default="../../discovery/dataset/rotation_captcha",
        help="Path to the rotation captcha dataset directory"
    )
    parser.add_argument("--batch-size", type=int, default=32, help="Batch size (e.g., 16 or 32)")
    parser.add_argument("--epochs", type=int, default=50, help="Number of epochs")
    parser.add_argument("--lr", type=float, default=1e-4, help="Learning rate")
    
    args = parser.parse_args()
    
    print(f"Using dataset directory: {args.dataset_dir}")
    
    dataset = RotationCaptchaDataset(args.dataset_dir)
    if len(dataset) == 0:
        print("No samples found. Check dataset path.")
        return
        
    print(f"Total samples found: {len(dataset)}")
    
    # Train / Val Split (80/20)
    train_size = int(0.8 * len(dataset))
    val_size = len(dataset) - train_size
    train_dataset, val_dataset = torch.utils.data.random_split(dataset, [train_size, val_size])
    
    print(f"Training samples: {train_size}, Validation samples: {val_size}")
    
    # DataLoaders setup
    train_loader = DataLoader(
        train_dataset, 
        batch_size=args.batch_size, 
        shuffle=True,  # Shuffle required for training
        num_workers=4, 
        pin_memory=True
    )
    
    val_loader = DataLoader(
        val_dataset, 
        batch_size=args.batch_size, 
        shuffle=False, 
        num_workers=4, 
        pin_memory=True
    )
    
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    print(f"Using device: {device}")
    
    model = get_model().to(device)
    
    criterion = nn.L1Loss()
    optimizer = torch.optim.Adam(model.parameters(), lr=args.lr)
    
    best_val_mae = float('inf')
    best_model_path = current_dir / "best_rotation_model.pth"
    
    print("Starting training...")
    
    for epoch in range(args.epochs):
        model.train()
        train_loss = 0.0
        
        for batch_idx, (inputs, targets) in enumerate(train_loader):
            if epoch == 0 and batch_idx == 0:
                print(f"First Batch - Input tensor shape: {inputs.shape}") # Validate [Batch, 6, 224, 224] shape
            
            inputs, targets = inputs.to(device), targets.to(device)
            
            optimizer.zero_grad()
            outputs = model(inputs)
            loss = criterion(outputs, targets)
            loss.backward()
            optimizer.step()
            
            train_loss += loss.item() * inputs.size(0)
            
        train_loss /= len(train_loader.dataset)
        
        # Validation Loop
        model.eval()
        val_loss = 0.0
        with torch.no_grad():
            for inputs, targets in val_loader:
                inputs, targets = inputs.to(device), targets.to(device)
                outputs = model(inputs)
                loss = criterion(outputs, targets)
                val_loss += loss.item() * inputs.size(0)
                
        val_loss /= len(val_loader.dataset)
        
        # Convert metric back to degrees for clearer visualization
        val_mae_degrees = val_loss * 360.0
        train_mae_degrees = train_loss * 360.0
        
        print(f"Epoch {epoch+1:03d}/{args.epochs:03d} | Train Error: {train_mae_degrees:.2f} graus | Val Error: {val_mae_degrees:.2f} graus")
        
        if val_mae_degrees < best_val_mae:
            best_val_mae = val_mae_degrees
            torch.save(model.state_dict(), best_model_path)
            print(f"  -> Best model saved at {best_model_path} (MAE: {best_val_mae:.2f} graus)")

    print("Training finished.")

if __name__ == "__main__":
    main()
