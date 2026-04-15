#!/usr/bin/env python3
"""
PDF Key Information Extractor

A reusable tool to extract key information from PDF files.
Can be called via: python pdf_extract.py <pdf_file> [options]

Usage:
    python pdf_extract.py <file.pdf>              # Extract all info
    python pdf_extract.py <file.pdf> --text      # Extract text only
    python pdf_extract.py <file.pdf> --metadata  # Extract metadata only
    python pdf_extract.py <file.pdf> --pages      # List pages info
    python pdf_extract.py <file.pdf> --search "keyword"  # Search in PDF
"""

import sys
import os
import argparse
from pathlib import Path

# Try multiple PDF libraries
PDFLIB_AVAILABLE = None

try:
    import PyPDF2
    PDFLIB_AVAILABLE = "PyPDF2"
except ImportError:
    try:
        import pdfminer.high_level
        import pdfminer.pdfparser
        import pdfminer.pdftypes
        PDFLIB_AVAILABLE = "pdfminer"
    except ImportError:
        pass

def extract_metadata(pdf_path):
    """Extract PDF metadata (author, title, creator, etc.)"""
    try:
        with open(pdf_path, 'rb') as f:
            if PDFLIB_AVAILABLE == "PyPDF2":
                reader = PyPDF2.PdfReader(f)
                info = reader.metadata
                return {
                    "title": info.get("/Title", "N/A") if info else "N/A",
                    "author": info.get("/Author", "N/A") if info else "N/A",
                    "subject": info.get("/Subject", "N/A") if info else "N/A",
                    "creator": info.get("/Creator", "N/A") if info else "N/A",
                    "producer": info.get("/Producer", "N/A") if info else "N/A",
                    "creation_date": info.get("/CreationDate", "N/A") if info else "N/A",
                    "modification_date": info.get("/ModDate", "N/A") if info else "N/A",
                }
            elif PDFLIB_AVAILABLE == "pdfminer":
                # pdfminer handling
                pass
    except Exception as e:
        return {"error": str(e)}
    return {"error": "No PDF library available. Run: pip install PyPDF2"}

def extract_text(pdf_path, pages=None):
    """Extract text from PDF"""
    try:
        with open(pdf_path, 'rb') as f:
            if PDFLIB_AVAILABLE == "PyPDF2":
                reader = PyPDF2.PdfReader(f)
                text_results = []
                
                if pages:
                    # Extract specific pages
                    for page_num in pages:
                        if 0 <= page_num < len(reader.pages):
                            text_results.append(f"--- Page {page_num + 1} ---\n")
                            text_results.append(reader.pages[page_num].extract_text())
                else:
                    # Extract all pages
                    for i, page in enumerate(reader.pages):
                        text_results.append(f"--- Page {i + 1} ---\n")
                        text_results.append(page.extract_text())
                
                return "\n".join(text_results)
            elif PDFLIB_AVAILABLE == "pdfminer":
                text = pdfminer.high_level.extract_text(pdf_path)
                return text
    except Exception as e:
        return f"Error: {str(e)}"
    return "Error: No PDF library available. Run: pip install PyPDF2"

def extract_pages_info(pdf_path):
    """Get information about each page"""
    try:
        with open(pdf_path, 'rb') as f:
            if PDFLIB_AVAILABLE == "PyPDF2":
                reader = PyPDF2.PdfReader(f)
                pages_info = []
                
                for i, page in enumerate(reader.pages):
                    pages_info.append({
                        "page_number": i + 1,
                        "page_count": len(reader.pages),
                    })
                
                return {
                    "total_pages": len(reader.pages),
                    "pages": pages_info
                }
    except Exception as e:
        return {"error": str(e)}
    return {"error": "No PDF library available. Run: pip install PyPDF2"}

def search_in_pdf(pdf_path, keyword):
    """Search for a keyword in PDF"""
    try:
        with open(pdf_path, 'rb') as f:
            if PDFLIB_AVAILABLE == "PyPDF2":
                reader = PyPDF2.PdfReader(f)
                results = []
                
                for i, page in enumerate(reader.pages):
                    text = page.extract_text()
                    if keyword.lower() in text.lower():
                        results.append({
                            "page": i + 1,
                            "found": True
                        })
                
                return {
                    "keyword": keyword,
                    "matches": len(results),
                    "pages_found": results
                }
    except Exception as e:
        return {"error": str(e)}
    return {"error": "No PDF library available. Run: pip install PyPDF2"}

def main():
    parser = argparse.ArgumentParser(description='PDF Key Information Extractor')
    parser.add_argument('pdf_file', help='Path to PDF file')
    parser.add_argument('--text', action='store_true', help='Extract text')
    parser.add_argument('--metadata', action='store_true', help='Extract metadata')
    parser.add_argument('--pages', action='store_true', help='List pages info')
    parser.add_argument('--search', type=str, help='Search keyword in PDF')
    parser.add_argument('--page', type=int, help='Extract specific page number')
    parser.add_argument('--output', type=str, help='Save output to file')
    parser.add_argument('--check-lib', action='store_true', help='Check available PDF library')
    
    args = parser.parse_args()
    
    # Check library
    if args.check_lib:
        if PDFLIB_AVAILABLE:
            print(f"✓ PDF library available: {PDFLIB_AVAILABLE}")
            return
        else:
            print("✗ No PDF library found. Installing...")
            print("Run: pip install PyPDF2")
            return
    
    # Check file exists
    if not os.path.exists(args.pdf_file):
        print(f"Error: File not found: {args.pdf_file}")
        return
    
    # Check library available
    if not PDFLIB_AVAILABLE:
        print("Error: No PDF library available.")
        print("Please install: pip install PyPDF2")
        sys.exit(1)
    
    # Process based on flags
    output = None
    
    if args.metadata:
        output = extract_metadata(args.pdf_file)
        for k, v in output.items():
            print(f"{k}: {v}")
        return
    
    if args.pages:
        output = extract_pages_info(args.pdf_file)
        print(f"Total Pages: {output.get('total_pages', 'N/A')}")
        return
    
    if args.search:
        output = search_in_pdf(args.pdf_file, args.search)
        print(f"Keyword: {output.get('keyword')}")
        print(f"Matches found: {output.get('matches', 0)}")
        for page in output.get('pages_found', []):
            print(f"  - Page {page['page']}")
        return
    
    if args.page:
        output = extract_text(args.pdf_path, pages=[args.page - 1])
        print(output)
        return
    
    # Default: extract all info
    print(f"PDF: {args.pdf_file}")
    print("=" * 50)
    
    # Metadata
    print("\n--- METADATA ---")
    metadata = extract_metadata(args.pdf_file)
    for k, v in metadata.items():
        if k != "error":
            print(f"{k}: {v}")
    
    # Pages info
    print("\n--- PAGES ---")
    pages_info = extract_pages_info(args.pdf_file)
    print(f"Total Pages: {pages_info.get('total_pages', 'N/A')}")
    
    # Text (first 3 pages as preview)
    print("\n--- TEXT PREVIEW (first 3 pages) ---")
    text = extract_text(args.pdf_file, pages=[0, 1, 2])
    print(text[:3000] if len(text) > 3000 else text)
    
    # Save to output file if specified
    if args.output:
        with open(args.output, 'w') as f:
            f.write(f"PDF: {args.pdf_file}\n")
            f.write("=" * 50 + "\n\n")
            f.write(f"Metadata: {metadata}\n")
            f.write(f"Pages: {pages_info}\n\n")
            f.write("Text:\n")
            f.write(text)

if __name__ == "__main__":
    main()
