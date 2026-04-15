# AI Task Instruction

## Information for AI: Best Tools for Reading PDF Files

You said that you have two excellent specialized tools for reading PDF files:

1. `extract_pdfs_full_content` (Recommended for Comprehensive Reading)
* Extracts complete content from PDFs for in-depth analysis
* Supports batch processing (multiple PDFs in one call)
* Maximum 30 pages per task
* Ideal when you need to understand the full document

2. `extract_pdfs_key_info` -> alternatively, I prepared python script: `pdf_extract.py`
* Extracts specific information using targeted prompts
* Perfect for finding particular data points, dates, names, statistics
* Supports batch processing (maximum 10 tasks per call)
* Great for screening/filtering multiple PDFs to identify relevant ones

In this Use Case (Multiple PDFs)
here's the recommended workflow:
1. **First pass** → Use `extract_pdfs_key_info` with a broad prompt like "What are the main topics covered in this document?" to quickly identify which PDFs are relevant
2. **Second pass** → Use `extract_pdfs_full_content` on the PDFs you need to study in depth

**Key Advantages**
| Feature               | Benefit                                      |
|----------------------|----------------------------------------------|
| Batch processing     | Process up to 10 PDFs simultaneously         |
| Targeted extraction  | Ask specific questions, get precise answers  |
| Full content mode    | Deep dive into complete documents            |
| Page range control   | Extract only relevant sections               |

## What to prepare first

* install python virtual environment in `venv` folder (if not already installed)
* install required packages in `requirements.txt` (if not already installed)

## Execution details

here you will act as **senior tech writer**, and senior QA

1. read PDF summary to understand the context: `_docs/markdown-results/summary/pdf-instruction-summary.md`
2. you will prepare an empty file: `_docs/markdown-results/summary/pdf-instruction.md`
3. use `pdf_extract.py` to open chunked PDF files one by one inside: `_docs/reference-pdf`
4. for each read pdf file, carefully read the content and capture the context in detail and store/update it in the markdown file you prepared in step 2
5. repeat this action until you read all the PDF files
